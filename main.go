/*
Quick and simple monitoring system

Usage:
 jsonmon path.to/config.yml
 jsonmon -v # Prints version to stdout and exits

Environment:
 HOST
  - defaults to localhost
  - the JSON API network interface
 PORT
  - defaults to 3000
  - the JSON API port
 GOMAXPROCS
  - defaults to CPU number + 1
  - number of threads to start
*/
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Application version
const Version = "0.9.6"

// Check details
type Check struct {
	Name   string      `json:"name,omitempty" yaml:"name"`
	Web    string      `json:"web,omitempty" yaml:"web"`
	Shell  string      `json:"shell,omitempty" yaml:"shell"`
	Notify interface{} `json:"-" yaml:"notify"`
	Repeat int         `json:"-" yaml:"repeat"`
	Failed bool        `json:"failed" yaml:"-"`
	Since  string      `json:"since,omitempty" yaml:"-"`
}

// Global checks list. Need to share it with workers and Web UI.
var checks []Check

// The main loop.
func main() {
	// Parse CLI args.
	version := flag.Bool("v", false, "print version to stdout and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:", path.Base(os.Args[0]), "path.to/config.yml")
		fmt.Fprintln(os.Stderr, "Docs:  https://github.com/chillum/jsonmon/wiki")
	}
	flag.Parse()
	// -v for version.
	if *version {
		fmt.Println("jsonmon", Version)
		fmt.Println(runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}
	// Should supply a config file.
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
	}
	// Tune concurrency.
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU() + 1)
	}
	// Read config file or exit with error.
	config, err := ioutil.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(3)
	}
	err = yaml.Unmarshal(config, &checks)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", config[0], err)
		os.Exit(3)
	}
	// Run checks.
	var wg sync.WaitGroup
	wg.Add(1)
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
	http.HandleFunc("/", display)
	err = http.ListenAndServe(host+":"+port, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
	}
	// Wait forever.
	wg.Wait()
}

// Background worker.
func worker(check *Check) {
	for {
		if check.Repeat == 0 { // Set default timeout.
			check.Repeat = 60
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
	// Set display name.
	var name string
	if check.Name != "" {
		name = check.Name
	} else {
		name = check.Shell
	}
	// Execute with shell.
	if out, err := exec.Command("/bin/sh", "-c", check.Shell).Output(); err != nil {
		if !check.Failed {
			check.Failed = true
			check.Since = time.Now().Format(time.RFC3339)
			alert(check.Notify, "FAILED: "+name, strings.TrimSpace(string(out)))
		}
	} else {
		if check.Failed {
			check.Failed = false
			check.Since = time.Now().Format(time.RFC3339)
			alert(check.Notify, "FIXED: "+name, "")
		}
	}
}

// Web worker.
func web(check *Check) {
	// Set display name.
	var name string
	if check.Name != "" {
		name = check.Name
	} else {
		name = check.Web
	}
	// Get the URL in 3 attempts.
	var err error
	for i := 0; i < 3; i++ {
		err = fetch(check.Web)
		if err == nil {
			break
		}
	}
	// Process status.
	if err == nil {
		if check.Failed {
			check.Failed = false
			check.Since = time.Now().Format(time.RFC3339)
			alert(check.Notify, "FIXED: "+name, "")
		}
	} else {
		if !check.Failed {
			check.Failed = true
			check.Since = time.Now().Format(time.RFC3339)
			alert(check.Notify, "FAILED: "+name, err.Error())
		}
	}
}

// The actual HTTP GET. Fails if HTTP status code is not 200.
func fetch(url string) error {
	resp, err := http.Get(url)
	if err == nil && resp.StatusCode != 200 {
		err = errors.New(url + " returned " + strconv.Itoa(resp.StatusCode))
	}
	if resp != nil {
		resp.Body.Close()
	}
	return err
}

// Logs and mail alerting.
func alert(mail interface{}, subject string, message string) {
	fmt.Println(subject)
	// Log the alerts.
	if message != "" {
		fmt.Println(message)
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
		msg := "To: " + rcpt + "\nSubject: " + subject + "\n\n"
		if message != "" {
			msg += message
		}
		msg += "\n.\n"
		// And send it.
		sendmail := exec.Command("/usr/sbin/sendmail", "-t")
		stdin, err := sendmail.StdinPipe()
		err = sendmail.Start()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ERROR:", err)
		}
		io.WriteString(stdin, msg)
		sendmail.Wait()
	}
}

// Format JSON for output.
func display(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" { // Serve for root page only, 404 otherwise.
		http.NotFound(w, r)
		return
	}
	json, _ := json.MarshalIndent(&checks, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.Write(json)
	fmt.Fprintln(w, "") // Trailing newline.
}
