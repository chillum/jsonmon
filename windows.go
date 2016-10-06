// +build windows

package main

import (
	"fmt"
	"golang.org/x/sys/windows/svc/eventlog"
	"os"
)
var logs *eventlog.Log

func logInit() (logwriter *eventlog.Log, err error) {
	logwriter, err = eventlog.Open("jsonmon")
	return
}

func log(severity int, message string) {
	if *useSyslog == false {
		fmt.Fprint(os.Stderr, "<", severity, ">", message, "\n")
	} else {
		switch severity {
		case 2:
			logs.Error(2, message)
		case 3:
			logs.Error(3, message)
		case 4:
			logs.Warning(4, message)
		case 5:
			logs.Info(5, message)
		case 7:
			logs.Info(7, message)
		}
	}
}
