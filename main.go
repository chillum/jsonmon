/*
Quick and simple monitoring system

Usage:
  jsonmon [--syslog] config.yml
  jsonmon --version

Docs: https://github.com/chillum/jsonmon/wiki
*/
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

	"gopkg.in/yaml.v2"
)

// Version is the application version.
const Version = "3.1.3"

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

// Global checks list. Need to share it with workers and Web UI.
var checks []Check

// Global started and last modified date for HTTP caching.
var modified string
var started string
var modHTML string
var modAngular string
var modJS string
var modCSS string

var mutex *sync.RWMutex

var useSyslog *bool

// Construct the last modified string.
func etag(ts time.Time) string {
	return "W/\"" + strconv.FormatInt(ts.UnixNano(), 10) + "\""
}

// The main loop.
func main() {
	var err error
	// Parse CLI args.
	cliVersion := flag.Bool("version", false, "")
	useSyslog = flag.Bool("syslog", false, "")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr,
			"Usage: ", path.Base(os.Args[0]), " [--syslog] config.yml\n",
			"       ", path.Base(os.Args[0]), " --version\n",
			"----------------------------------------------\n",
			"Docs:  https://github.com/chillum/jsonmon/wiki\n")
		os.Exit(1)
	}
	flag.Parse()

	// -v for version.
	version.App = Version
	version.Go = runtime.Version()
	version.Os = runtime.GOOS
	version.Arch = runtime.GOARCH

	if *cliVersion {
		json, _ := json.Marshal(&version)
		fmt.Println(string(json))
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
	}

	if *useSyslog == true {
		logs, err = logInit()
		if err != nil {
			*useSyslog = false
			log(3, "Syslog failed, disabling: "+err.Error())
		}
	}

	// Parse the config file or exit with error.
	config, err := ioutil.ReadFile(args[0])
	if err != nil {
		log(2, err.Error())
		os.Exit(3)
	}
	err = yaml.Unmarshal(config, &checks)
	if err != nil {
		log(2, "invalid config at "+os.Args[1]+"\n"+err.Error())
		os.Exit(3)
	}

	// Exit with return code 0 on kill.
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM)
	go func() {
		<-done
		os.Exit(0)
	}()

	// Run checks and init HTTP cache.
	started = etag(time.Now())
	modified = started
	mutex = &sync.RWMutex{}
	for i := range checks {
		go worker(&checks[i])
	}
	cacheHTML, _ := AssetInfo("index.html")
	modHTML = cacheHTML.ModTime().UTC().Format(http.TimeFormat)
	cacheAngular, _ := AssetInfo("angular.min.js")
	modAngular = cacheAngular.ModTime().UTC().Format(http.TimeFormat)
	cacheJS, _ := AssetInfo("app.js")
	modJS = cacheJS.ModTime().UTC().Format(http.TimeFormat)
	cacheCSS, _ := AssetInfo("main.css")
	modCSS = cacheCSS.ModTime().UTC().Format(http.TimeFormat)

	// Launch the Web server.
	host := os.Getenv("HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	listen := host + ":" + port

	http.HandleFunc("/status", getChecks)
	http.HandleFunc("/version", getVersion)
	http.HandleFunc("/", getUI)

	log(7, "Starting HTTP service at "+listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		log(2, err.Error())
		log(7, "Use HOST and PORT env variables to customize server settings")
	}
	os.Exit(4)
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
		out, err = exec.Command("sh", "-c", check.Shell).CombinedOutput()
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
				notify(check, &subject, nil)
			}
			if check.Alert != "" {
				alert(check, name, nil, false)
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
				notify(check, &subject, &msg)
			}
			if check.Alert != "" {
				alert(check, name, &msg, true)
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
				notify(check, &subject, nil)
			}
			if check.Alert != "" {
				alert(check, name, nil, false)
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
				notify(check, &subject, &msg)
			}
			if check.Alert != "" {
				alert(check, name, &msg, true)
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

// Mail notifications.
func notify(check *Check, subject *string, message *string) {
	// Make the message.
	var msg bytes.Buffer
	var err error
	msg.WriteString("To: ")
	msg.WriteString(check.Notify)
	msg.WriteString("\nSubject: ")
	msg.WriteString(*subject)
	msg.WriteString("\nX-Mailer: jsonmon\n\n")
	if message != nil {
		msg.WriteString(*message)
	}
	msg.WriteString("\n.\n")
	// And send it.
	sendmail := exec.Command("/usr/sbin/sendmail", "-t")
	stdin, _ := sendmail.StdinPipe()
	err = sendmail.Start()
	if err != nil {
		log(3, err.Error())
	}
	io.WriteString(stdin, msg.String())
	sendmail.Wait()
}

// Executes callback. Passes args: true/false, check's name, message.
func alert(check *Check, name *string, msg *string, failed bool) {
	var out []byte
	var err error
	if msg != nil {
		out, err = exec.Command(check.Alert, strconv.FormatBool(failed), *name, *msg).CombinedOutput()
	} else {
		out, err = exec.Command(check.Alert, strconv.FormatBool(failed), *name).CombinedOutput()
	}
	if err != nil {
		log(3, check.Alert+" failed\n"+string(out)+err.Error())
	}
}

// Serve the Web UI.
func getUI(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Server", "jsonmon")
	switch r.URL.Path {
	case "/":
		displayUI(w, r, "text/html", "index.html", &modHTML)
	case "/angular.min.js":
		displayUI(w, r, "application/javascript", "angular.min.js", &modAngular)
	case "/app.js":
		displayUI(w, r, "application/javascript", "app.js", &modJS)
	case "/main.css":
		displayUI(w, r, "text/css", "main.css", &modCSS)
	default:
		http.NotFound(w, r)
	}
}

// Web UI caching and delivery.
func displayUI(w http.ResponseWriter, r *http.Request, mime string, name string, modified *string) {
	if cached := r.Header.Get("If-Modified-Since"); cached == *modified {
		w.WriteHeader(http.StatusNotModified)
	} else {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Last-Modified", *modified)
		h.Set("Content-Type", mime)
		data, _ := Asset(name)
		w.Write(data)
	}
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
