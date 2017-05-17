// +build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc/eventlog"
)

// ShellPath points to a Bourne-compatible shell.
// We expect `sh` anywhere in %PATH% on Windows.
const ShellPath string = "sh"

type WindowsLogger struct {
	eventLog *eventlog.Log
}

func init() {
	var eventLogger *eventlog.Log
	if syslogFlag {
		var err error
		eventLogger, err = eventlog.Open(AppName)
		if err != nil {
			fatal(ErrorIO, err.Error())
		}
	}
	logs = &WindowsLogger{eventLogger}
}

func (l *WindowsLogger) Log(severity int, message string) {
	if severity > logLevel {
		return
	}
	if l.eventLog == nil {
		msg := fmt.Sprintf(logFormat, ToLogLevel(severity), message)
		if severity >= LOG_INFO {
			fmt.Fprintln(os.Stdout, msg)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
	} else {
		switch severity {
		case LOG_CRIT:
			fallthrough
		case LOG_ERR:
			l.eventLog.Error(uint32(LOG_ERR), message)
		case LOG_WARNING:
			l.eventLog.Warning(uint32(LOG_WARNING), message)
		case LOG_NOTICE:
			fallthrough
		case LOG_INFO:
			fallthrough
		case LOG_DEBUG:
			l.eventLog.Info(uint32(severity), message)
		}
	}
}
