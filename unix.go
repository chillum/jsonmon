// +build !windows

package main

import (
	"fmt"
	"log/syslog"
	"os"
)

// ShellPath points to a Bourne-compatible shell.
// /bin/sh is the standard path that should work on any Unix.
const ShellPath string = "/bin/sh"

type UnixLogger struct {
	writer *syslog.Writer
}

func init() {
	var syslogWriter *syslog.Writer
	if syslogFlag {
		var err error
		syslogWriter, err = syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, AppName)
		if err != nil {
			fatal(ErrorIO, err.Error())
		}
	}
	logs = &UnixLogger{syslogWriter}
}

func (l *UnixLogger) Log(severity int, message string) {
	if severity > logLevel {
		return
	}
	if l.writer == nil {
		msg := fmt.Sprintf(logFormat, ToLogLevel(severity), message)
		if severity >= LOG_INFO {
			fmt.Fprintln(os.Stdout, msg)
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
	} else {
		switch severity {
		case LOG_CRIT:
			l.writer.Crit(message)
		case LOG_ERR:
			l.writer.Err(message)
		case LOG_WARNING:
			l.writer.Warning(message)
		case LOG_NOTICE:
			l.writer.Notice(message)
		case LOG_INFO:
			l.writer.Notice(message)
		case LOG_DEBUG:
			l.writer.Debug(message)
		}
	}
}
