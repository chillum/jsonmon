/*
Quick and simple monitoring system

Usage:
 jsonmon config.yml
 jsonmon -v # Prints version to stdout and exits

Environment:
 HOST: the JSON API network interface, defaults to localhost
 PORT: the JSON API port, defaults to 3000

More docs: https://github.com/chillum/jsonmon/wiki
*/
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Version is the application version.
const Version = "2.0.7"

// This one is for internal use.
type ver struct {
	App  string `json:"jsonmon"`
	Go   string `json:"runtime"`
	Os   string `json:"os"`
	Arch string `json:"arch"`
}

var version ver

// Check details.
type Check struct {
	Name   string      `json:"name,omitempty" yaml:"name"`
	Web    string      `json:"web,omitempty" yaml:"web"`
	Shell  string      `json:"shell,omitempty" yaml:"shell"`
	Match  string      `json:"-" yaml:"match"`
	Return int         `json:"-" yaml:"return"`
	Notify interface{} `json:"-" yaml:"notify"`
	Alert  interface{} `json:"-" yaml:"alert"`
	Tries  int         `json:"-" yaml:"tries"`
	Repeat int         `json:"-" yaml:"repeat"`
	Failed bool        `json:"failed" yaml:"-"`
	Since  string      `json:"since,omitempty" yaml:"-"`
}

// Global checks list. Need to share it with workers and Web UI.
var checks []Check

// Global started and last modified date for HTTP caching.
var modified string
var started string

var mutex *sync.RWMutex

// Construct the last modified string.
func etag(ts time.Time) string {
	return "W/\"" + strconv.FormatInt(ts.UnixNano(), 10) + "\""
}

// The main loop.
func main() {
	// Parse CLI args.
	usage := "Usage: " + path.Base(os.Args[0]) + " config.yml\n" +
		"Docs:  https://github.com/chillum/jsonmon/wiki"
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
	// -v for version.
	version.App = Version
	version.Go = runtime.Version()
	version.Os = runtime.GOOS
	version.Arch = runtime.GOARCH
	switch os.Args[1] {
	case "-h":
		fallthrough
	case "-help":
		fallthrough
	case "--help":
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(0)
	case "-v":
		fallthrough
	case "-version":
		fallthrough
	case "--version":
		json, _ := json.Marshal(&version)
		fmt.Println(string(json))
		os.Exit(0)
	}
	// Read config file or exit with error.
	config, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprint(os.Stderr, "<2>", err, "\n")
		os.Exit(3)
	}
	err = yaml.Unmarshal(config, &checks)
	if err != nil {
		fmt.Fprint(os.Stderr, "<2>", "invalid config at ", os.Args[1], "\n")
		os.Exit(3)
	}
	// Exit with return code 0 on kill.
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM)
	go func() {
		<-done
		os.Exit(0)
	}()
	// Run checks.
	started = etag(time.Now())
	modified = started
	mutex = &sync.RWMutex{}
	for i := range checks {
		go worker(&checks[i])
	}
	// Launch the JSON API.
	host := os.Getenv("HOST")
	port := os.Getenv("PORT")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "3000"
	}
	http.HandleFunc("/status", getChecks)
	http.HandleFunc("/version", getVersion)
	http.HandleFunc("/", notFound)
	err = http.ListenAndServe(host+":"+port, nil)
	if err != nil {
		fmt.Fprint(os.Stderr, "<2>", err, "\n")
	}
	os.Exit(4)
}

// Background worker.
func worker(check *Check) {
	mutex.Lock()
	if check.Repeat == 0 { // Set default timeout.
		check.Repeat = 30
	}
	if check.Tries == 0 { // Default to 1 attempt.
		check.Tries = 1
	}
	mutex.Unlock()
	sleep := time.Second * time.Duration(check.Repeat)
	for {
		if check.Web != "" {
			web(check)
		}
		if check.Shell != "" {
			shell(check)
		}
		time.Sleep(sleep)
	}
}

// Shell worker.
func shell(check *Check) {
	// Set check's display name.
	var name string
	if check.Name != "" {
		name = check.Name
	} else {
		name = check.Shell
	}
	// Execute with shell in N attemps.
	var out []byte
	var err error
	for i := 0; i < check.Tries; i++ {
		out, err = exec.Command("/bin/sh", "-c", check.Shell).CombinedOutput()
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
			notify(check.Notify, "Fixed: "+name, nil)
			alert(check, &name, nil)
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
			notify(check.Notify, "Failed: "+name, &msg)
			alert(check, &name, &msg)
		}
	}
}

// Web worker.
func web(check *Check) {
	// Set check's display name.
	var name string
	if check.Name != "" {
		name = check.Name
	} else {
		name = check.Web
	}
	if check.Return == 0 { // Successful HTTP return code is 200.
		check.Return = 200
	}
	// Get the URL in N attempts.
	var err error
	for i := 0; i < check.Tries; i++ {
		err = fetch(check.Web, check.Match, check.Return)
		if err == nil {
			break
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
			notify(check.Notify, "Fixed: "+name, nil)
			alert(check, &name, nil)

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
			notify(check.Notify, "Failed: "+name, &msg)
			alert(check, &name, &msg)
		}
	}
}

// Check HTTP redirects.
func redirect(req *http.Request, via []*http.Request) error {
	// When redirects number > 10 probably there's a problem.
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	// Redirects don't get User-Agent.
	req.Header.Set("User-Agent", "jsonmon")
	return nil
}

// The actual HTTP GET.
func fetch(url string, match string, code int) error {
	var err error
	var resp *http.Response
	var req *http.Request
	client := &http.Client{}
	client.CheckRedirect = redirect
	req, err = http.NewRequest("GET", url, nil)
	if err == nil {
		req.Header.Set("User-Agent", "jsonmon")
		resp, err = client.Do(req)
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
	}
	if resp != nil {
		resp.Body.Close()
	}
	return err
}

// Logs and mail alerting.
func notify(mail interface{}, subject string, message *string) {
	// Log the alerts.
	if message == nil {
		fmt.Print("<5>", subject, "\n")
	} else {
		fmt.Print("<5>", subject, "\n", *message, "\n")
	}
	// Mail the alerts.
	if mail != nil {
		// Make the message.
		var rcpt string
		var ok bool
		if rcpt, ok = mail.(string); !ok {
			for i, v := range mail.([]interface{}) {
				if i != 0 {
					rcpt += ", "
				}
				rcpt += v.(string)
			}
		}
		msg := "To: " + rcpt + "\nSubject: " + subject + "\nX-Mailer: jsonmon\n\n"
		if message != nil {
			msg += *message
		}
		msg += "\n.\n"
		// And send it.
		sendmail := exec.Command("/usr/sbin/sendmail", "-t")
		stdin, _ := sendmail.StdinPipe()
		err := sendmail.Start()
		if err != nil {
			fmt.Fprint(os.Stderr, "<3>", err, "\n")
		}
		io.WriteString(stdin, msg)
		sendmail.Wait()
	}
}

// Executes callback. Passes args: true/false, check's name, message.
func alert(check *Check, name *string, msg *string) {
	if check.Alert != nil {
		plugin, ok := check.Alert.(string)
		if ok { // check.Alert is a string.
			out, err := exec.Command(plugin, strconv.FormatBool(check.Failed), *name, *msg).CombinedOutput()
			if err != nil {
				fmt.Fprint(os.Stderr, "<3>", plugin, " failed\n", string(out), err.Error(), "\n")
			}
		} else { // check.Alert is a list.
			for _, i := range check.Alert.([]interface{}) {
				out, err := exec.Command(i.(string), strconv.FormatBool(check.Failed), *name, *msg).CombinedOutput()
				if err != nil {
					fmt.Fprint(os.Stderr, "<3>", i, " failed\n", string(out), err.Error(), "\n")
				}
			}
		}
	}
}

// 404 error.
func notFound(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Server", "jsonmon")
	http.NotFound(w, r)
}

// Display checks' details.
func getChecks(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &checks, &modified, true)
}

// Display application version.
func getVersion(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &version, &started, false)
}

// Output JSON.
func displayJSON(w http.ResponseWriter, r *http.Request, data interface{}, cache *string, lock bool) {
	var cached bool
	var result []byte
	h := w.Header()
	h.Set("Server", "jsonmon")
	if lock {
		mutex.RLock()
	}
	if r.Header.Get("If-None-Match") == *cache {
		cached = true
	} else {
		h.Set("ETag", *cache)
		result, _ = json.Marshal(&data)
	}
	if lock {
		mutex.RUnlock()
	}
	if cached {
		w.WriteHeader(http.StatusNotModified)
	} else {
		h.Set("Cache-Control", "no-cache")
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Type", "application/json; charset=utf-8")
		w.Write(result)
	}
}
