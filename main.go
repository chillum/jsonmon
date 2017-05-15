/*
Quick and simple monitoring system

Usage:
  jsonmon [-syslog] config.yml
  jsonmon -version

Docs: https://github.com/chillum/jsonmon/wiki
*/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	BindAddress = "0.0.0.0"
	BindPort    = "3000"
)

// Global checks list. Need to share it with workers and Web UI.
var checks []*Check

// Global started and last modified date for HTTP caching.
var modified string
var started string
var modHTML string
var modAngular string
var modJS string
var modCSS string

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
			"Usage: ", path.Base(os.Args[0]), " [-syslog] config.yml\n",
			"       ", path.Base(os.Args[0]), " -version\n",
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
	for _, check := range checks {
		go check.Run()
	}

	started = etag(time.Now())
	modified = started

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
		host = BindAddress
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = BindPort
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
	displayJSON(w, r, &checks, &modified)
}

// Display application version.
func getVersion(w http.ResponseWriter, r *http.Request) {
	displayJSON(w, r, &version, &started)
}

// Output JSON.
func displayJSON(w http.ResponseWriter, r *http.Request, data interface{}, cache *string) {
	var cached bool
	var result []byte
	h := w.Header()
	h.Set("Server", "jsonmon")
	if r.Header.Get("If-None-Match") == *cache {
		cached = true
	} else {
		h.Set("ETag", *cache)
		result, _ = json.Marshal(&data)
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
