package main

import (
	"errors"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// Check details.
type Check struct {
	Name   string `json:"name,omitempty"`
	Web    string `json:"web,omitempty"`
	Shell  string `json:"shell,omitempty"`
	Match  string `json:"-"`
	Return int    `json:"-"`
	Notify string `json:"-"`
	Alert  string `json:"-"`
	Tries  int    `json:"-"`
	Repeat int    `json:"-"`
	Sleep  int    `json:"-"`
	Failed bool   `json:"failed" yaml:"-"`
	Since  string `json:"since,omitempty" yaml:"-"`
}

// Background worker.
func worker(check *Check) {
	if check.Shell == "" && check.Web == "" {
		log(4, "Ignoring entry with no either Web or shell check")
		mutex.Lock()
		check.Failed = true
		mutex.Unlock()
		return
	}
	if check.Shell != "" && check.Web != "" {
		log(3, "Web and shell checks in one block are not allowed")
		log(3, "Disabled: "+check.Shell)
		log(3, "Disabled: "+check.Web)
		mutex.Lock()
		check.Failed = true
		mutex.Unlock()
		return
	}
	mutex.Lock()
	if check.Repeat == 0 { // Set default timeout.
		check.Repeat = 30
	}
	if check.Tries == 0 { // Default to 1 attempt.
		check.Tries = 1
	}
	mutex.Unlock()
	repeat := time.Second * time.Duration(check.Repeat)
	sleep := time.Second * time.Duration(check.Sleep)
	var name string
	if check.Web != "" {
		if check.Name != "" { // Set check's display name.
			name = check.Name
		} else {
			name = check.Web
		}
		if check.Return == 0 { // Successful HTTP return code is 200.
			mutex.Lock()
			check.Return = 200
			mutex.Unlock()
		}
		for {
			web(check, &name, &sleep)
			time.Sleep(repeat)
		}
	} else {
		if check.Name != "" { // Set check's display name.
			name = check.Name
		} else {
			name = check.Shell
		}
		for {
			shell(check, &name, &sleep)
			time.Sleep(repeat)
		}
	}
}

// Shell worker.
func shell(check *Check, name *string, sleep *time.Duration) {
	// Execute with shell in N attemps.
	var out []byte
	var err error
	for i := 0; i < check.Tries; {
		out, err = exec.Command(ShellPath, "-c", check.Shell).CombinedOutput()
		if err == nil {
			if check.Match != "" { // Match regexp.
				var regex *regexp.Regexp
				regex, err = regexp.Compile(check.Match)
				if err == nil && !regex.Match(out) {
					err = errors.New("Expected:\n" + check.Match + "\n\nGot:\n" + string(out))
				}
			}
			break
		}
		i++
		if i < check.Tries {
			time.Sleep(*sleep)
		}
	}
	// Process results.
	if err == nil {
		if check.Failed {
			ts := time.Now()
			mutex.Lock()
			check.Failed = false
			check.Since = ts.Format(time.RFC3339)
			modified = etag(ts)
			mutex.Unlock()
			subject := "Fixed: " + *name
			log(5, subject)
			if check.Notify != "" {
				go notify(check, &subject, nil)
			}
			if check.Alert != "" {
				go alert(check, name, nil, false)
			}
		}
	} else {
		if !check.Failed {
			ts := time.Now()
			mutex.Lock()
			check.Failed = true
			check.Since = ts.Format(time.RFC3339)
			modified = etag(ts)
			mutex.Unlock()
			msg := string(out) + err.Error()
			subject := "Failed: " + *name
			log(5, subject+"\n"+msg)
			if check.Notify != "" {
				go notify(check, &subject, &msg)
			}
			if check.Alert != "" {
				go alert(check, name, &msg, true)
			}
		}
	}
}

// Web worker.
func web(check *Check, name *string, sleep *time.Duration) {
	// Get the URL in N attempts.
	var err error
	for i := 0; i < check.Tries; {
		err = fetch(check.Web, check.Match, check.Return)
		if err == nil {
			break
		}
		i++
		if i < check.Tries {
			time.Sleep(*sleep)
		}
	}
	// Process results.
	if err == nil {
		if check.Failed {
			ts := time.Now()
			mutex.Lock()
			check.Failed = false
			check.Since = ts.Format(time.RFC3339)
			modified = etag(ts)
			mutex.Unlock()
			subject := "Fixed: " + *name
			log(5, subject)
			if check.Notify != "" {
				go notify(check, &subject, nil)
			}
			if check.Alert != "" {
				go alert(check, name, nil, false)
			}
		}
	} else {
		if !check.Failed {
			ts := time.Now()
			mutex.Lock()
			check.Failed = true
			check.Since = ts.Format(time.RFC3339)
			modified = etag(ts)
			mutex.Unlock()
			msg := err.Error()
			subject := "Failed: " + *name
			log(5, subject+"\n"+msg)
			if check.Notify != "" {
				go notify(check, &subject, &msg)
			}
			if check.Alert != "" {
				go alert(check, name, &msg, true)
			}
		}
	}
}

// The actual HTTP GET.
func fetch(url string, match string, code int) error {
	resp, err := http.Get(url)
	if err == nil {
		if resp.StatusCode != code { // Check status code.
			err = errors.New(url + " returned " + strconv.Itoa(resp.StatusCode))
		} else { // Match regexp.
			if resp != nil && match != "" {
				var regex *regexp.Regexp
				regex, err = regexp.Compile(match)
				if err == nil {
					var body []byte
					body, _ = ioutil.ReadAll(resp.Body)
					if !regex.Match(body) {
						err = errors.New("Expected:\n" + match + "\n\nGot:\n" + string(body))
					}
				}
			}
		}
	}
	if resp != nil {
		resp.Body.Close()
	}
	return err
}
