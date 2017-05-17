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
	"syscall"

	"gopkg.in/yaml.v2"
)

const (
	BindAddress = "0.0.0.0"
	BindPort    = "3000"
	AppName     = "jsonmon"
)

// Context related exit codes
const (
	ErrorIO = iota
	ErrorMarshal
	ErrorArguments
	ErrorNet
)

// Global checks list. Need to share it with workers and Web UI.
var checks Checks

func main() {
	if versionFlag {
		json, _ := json.Marshal(NewVersionPayload())
		fmt.Println(string(json))
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) != 1 {
		flag.Usage()
	}

	// Parse the config file or exit with error.
	config, err := ioutil.ReadFile(args[0])
	if err != nil {
		fatal(ErrorIO, err.Error())
	}
	err = yaml.Unmarshal(config, &checks)
	if err != nil {
		fatal(ErrorMarshal, fmt.Sprintf("Invalid config at %q with error:\n\t%v", os.Args[1], err))
	}

	// Listening to SIGTERM signal and exit with return code 0 on kill.
	go func() {
		done := make(chan os.Signal, 1)
		signal.Notify(done, syscall.SIGTERM)
		<-done
		os.Exit(0)
	}()

	// initialize checks
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

	logs.Log(LOG_INFO, "Starting HTTP service at "+listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		fatal(ErrorNet, err.Error())
	}
}

// print the err message and exit with given exit code
func fatal(code int, err string) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(code)
}
