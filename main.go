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
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"syscall"

	"gopkg.in/yaml.v2"
)

const (
	BindAddress = "0.0.0.0"
	BindPort    = "3000"
)

// Global checks list. Need to share it with workers and Web UI.
var checks []*Check

var useSyslog *bool

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

	// Launch the Web server.
	host := os.Getenv("HOST")
	if host == "" {
		host = BindAddress
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = BindPort
	}
	listen := net.JoinHostPort(host, port)

	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/version", handleVersion)
	http.HandleFunc("/", handleUI)

	log(7, "Starting HTTP service at "+listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		log(2, err.Error())
		log(7, "Use HOST and PORT env variables to customize server settings")
	}
	os.Exit(4)
}
