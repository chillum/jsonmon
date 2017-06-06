package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// Check details.
type Check struct {
	Name    string            `json:"name,omitempty"`
	Web     string            `json:"web,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Shell   string            `json:"shell,omitempty"`
	Match   string            `json:"-"`
	Return  int               `json:"-"`
	Notify  string            `json:"-"`
	Alert   string            `json:"-"`
	Tries   int               `json:"-"`
	Repeat  int               `json:"-"`
	Sleep   int               `json:"-"`
	Failed  bool              `json:"failed" yaml:"-"`
	Since   string            `json:"since,omitempty" yaml:"-"`
}

// Run the check's loop.
func (check *Check) Run() {
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
			name = check.Web // TODO: strip http(s):// and basic auth
		}
		if check.Return == 0 { // Successful HTTP return code is 200.
			mutex.Lock()
			check.Return = 200
			mutex.Unlock()
		}
		for {
			check.web(&name, &sleep)
			time.Sleep(repeat)
		}
	} else {
		if check.Name != "" { // Set check's display name.
			name = check.Name
		} else {
			name = check.Shell
		}
		for {
			check.shell(&name, &sleep)
			time.Sleep(repeat)
		}
	}
}

// Shell worker.
func (check *Check) shell(name *string, sleep *time.Duration) {
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
				go notify(&check.Notify, &subject, nil)
			}
			if check.Alert != "" {
				go alert(&check.Alert, name, nil, false)
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
				go notify(&check.Notify, &subject, &msg)
			}
			if check.Alert != "" {
				go alert(&check.Alert, name, &msg, true)
			}
		}
	}
}

// Web worker.
func (check *Check) web(name *string, sleep *time.Duration) {
	// Get the URL in N attempts.
	var err error
	for i := 0; i < check.Tries; {
		err = check.fetch()
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
				go notify(&check.Notify, &subject, nil)
			}
			if check.Alert != "" {
				go alert(&check.Alert, name, nil, false)
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
				go notify(&check.Notify, &subject, &msg)
			}
			if check.Alert != "" {
				go alert(&check.Alert, name, &msg, true)
			}
		}
	}
}

// The actual HTTP GET.
func (check *Check) fetch() error {
	var resp *http.Response
	var err error
	method := "GET"
	if len(check.Headers) > 0 {
		client := http.Client{}
		var body io.Reader
		if len(check.Body) > 0 {
			method = "POST"
			body = bytes.NewReader([]byte(check.Body))
		}
		req, err := http.NewRequest(method, check.Web, body)
		if err != nil {
			return err
		}
		for k, v := range check.Headers {
			req.Header.Add(k, v)
		}
		resp, err = client.Do(req)
	} else {
		resp, err = http.Get(check.Web)
	}
	if err == nil {
		if resp.StatusCode != check.Return { // Check status code.
			err = errors.New(check.Web + " returned " + strconv.Itoa(resp.StatusCode))
		} else { // Match regexp.
			if resp != nil && check.Match != "" {
				var regex *regexp.Regexp
				regex, err = regexp.Compile(check.Match)
				if err == nil {
					var body []byte
					body, _ = ioutil.ReadAll(resp.Body)
					if !regex.Match(body) {
						err = errors.New("Expected:\n" + check.Match + "\n\nGot:\n" + string(body))
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
