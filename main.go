/*
Quick and simple monitoring system

Usage:
 jsonmon config.yml
 jsonmon -v # Prints version to stdout and exits

Environment:
 HOST
  - defaults to localhost
  - the JSON API network interface
 PORT
  - defaults to 3000
  - the JSON API port
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
	"syscall"
	"time"
)

// Application version.
const Version = "2.0.2"

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

// Construct the last modified string.
func etag() string{
	return "W/\"" + strconv.FormatInt(time.Now().UnixNano(), 10) + "\""
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
	// Tune concurrency.
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
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
	started = etag()
	modified = started
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
	for {
		if check.Repeat == 0 { // Set default timeout.
			check.Repeat = 30
		}
		if check.Tries == 0 { // Default to 1 attempt.
			check.Tries = 1
		}
		if check.Web != "" {
			web(check)
		}
		if check.Shell != "" {
			shell(check)
		}
		time.Sleep(time.Second * time.Duration(check.Repeat))
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
			check.Failed = false
			check.Since = time.Now().Format(time.RFC3339)
			modified = etag()
			notify(check.Notify, "Fixed: "+name, nil)
			alert(check, &name, nil)
		}
	} else {
		if !check.Failed {
			check.Failed = true
			check.Since = time.Now().Format(time.RFC3339)
			modified = etag()
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
			check.Failed = false
			check.Since = time.Now().Format(time.RFC3339)
			modified = etag()
			notify(check.Notify, "Fixed: "+name, nil)
			alert(check, &name, nil)

		}
	} else {
		if !check.Failed {
			check.Failed = true
			check.Since = time.Now().Format(time.RFC3339)
			modified = etag()
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
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-XSS-Protection", "1; mode=block")
	http.NotFound(w, r)
}

// Display checks' details.
func getChecks(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &checks, &modified)
}

// Display application version.
func getVersion(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &version, &started)
}

// Output JSON.
func displayJSON(w http.ResponseWriter, r *http.Request, data interface{}, cache *string) {
	h := w.Header()
	h.Set("Server", "jsonmon")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-XSS-Protection", "1; mode=block")
	h.Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == *cache {
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(http.StatusNotModified)
	} else {
		h.Set("Cache-Control", "no-cache")
		h.Set("ETag", *cache)
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Content-Type", "application/json; charset=utf-8")
		json, _ := json.Marshal(&data)
		w.Write(json)
	}
}
