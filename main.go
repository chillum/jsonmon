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
)

// Global checks list. Need to share it with workers and Web UI.
var checks []*Check

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

	if syslogFlag {
		var err error
		logs, err = logInit()
		if err != nil {
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
		log(2, fmt.Sprintf("Invalid config at %q with error:\n\t%v", os.Args[1], err))
		os.Exit(3)
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

	log(7, "Starting HTTP service at "+listen)
	err = http.ListenAndServe(listen, nil)
	if err != nil {
		log(2, err.Error())
		log(7, "Use HOST and PORT env variables to customize server settings")
	}
	os.Exit(4)
}
