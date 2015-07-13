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
  - number of Golang processes to start
*/
package main

import (
	"fmt"
	flag "github.com/ogier/pflag"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Application version
const Version = "0.9.3"

// Check details
type Check struct {
	Name   string `json:"name" yaml:"name"`
	Web    string `json:"web" yaml:"web"`
	Shell  string `json:"shell" yaml:"shell"`
	Notify string `yaml:"notify"`
	Repeat int    `yaml:"repeat"`
	Failed bool   `json:"failed"`
	Since  string `json:"since"`
}

// The main loop.
func main() {
	// Parse CLI args.
	version := flag.BoolP("version", "v", false, "print version to stdout and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:", os.Args[0], "path.to/config.yml")
		fmt.Fprintln(os.Stderr, "Docs:\n  https://github.com/chillum/jsonmon/wiki")
	}
	flag.Parse()

	if *version {
		fmt.Println("jsonmon", Version)
		fmt.Println(runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	config := flag.Args()
	if len(config) < 1 {
		flag.Usage()
	}

	// Tune concurrency.
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU() + 1)
	}

	// Read YAML config or exit with an error.
	yml, err := ioutil.ReadFile(config[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(3)
	}
	var checks []Check
	err = yaml.Unmarshal(yml, &checks)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", config[0], err)
		os.Exit(3)
	}

	// Run checks.
	var wg sync.WaitGroup
	wg.Add(1)
	for _, check := range checks {
		go worker(check)
	}
	// TODO: Launch the JSON API.
	wg.Wait()
}

func worker(check Check) {
	for {
		if check.Repeat == 0 { // Set default timeout.
			check.Repeat = 60
		}
		if check.Web != "" {
			web(&check)
		}
		if check.Shell != "" {
			shell(&check)
		}
		time.Sleep(time.Second * time.Duration(check.Repeat))
	}
}

// Shell worker.
func shell(check *Check) {
	// Display name.
	var name string
	if check.Name != "" {
		name = check.Name
	} else {
		name = check.Shell
	}

	// Execute with shell.
	if out, err := exec.Command("sh", "-c", check.Shell).Output(); err != nil {
		if !check.Failed {
			check.Failed = true
			alert(&check.Notify, "FAILED: "+name, strings.TrimSpace(string(out)))
		}
	} else {
		if check.Failed {
			check.Failed = false
			alert(&check.Notify, "FIXED: "+name, "")
		}
	}
}

// Web worker.
func web(check *Check) {
	// Display name.
	var name string
	if check.Name != "" {
		name = check.Name
	} else {
		name = check.Web
	}

	// Get the URL. TODO: 3 attempts
	resp, err := http.Get(check.Web)

	if err != nil {
		if !check.Failed {
			check.Failed = true
			alert(&check.Notify, "FAILED: "+name, err.Error())
		}
	} else {
		if check.Failed {
			check.Failed = false
			alert(&check.Notify, "FIXED: "+name, "")
		}
	}

	if resp != nil {
		resp.Body.Close()
	}
}

// Logs and mail alerting.
// TODO: send mail
func alert(mail *string, subject string, message string) {
	fmt.Println(subject)
	if message != "" {
		fmt.Println(message)
	}
}
