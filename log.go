package main

import (
	"os"
	"regexp"
)

const (
	// Severity.

	// From /usr/include/sys/syslog.h.
	// These are the same on Linux, BSD, and OS X.
	LOG_EMERG int = iota
	LOG_ALERT
	LOG_CRIT
	LOG_ERR
	LOG_WARNING
	LOG_NOTICE
	LOG_INFO
	LOG_DEBUG
)

const (
	EnvLogLevel      = "LOG_LEVEL"
	EnvLogFormat     = "LOG_FORMAT"
	defaultLogFormat = "<%s>\t%s" // #1 severity, #2 msg
)

type Logger interface {
	Log(severity int, message string)
}

var logs Logger
var logLevel = LOG_INFO
var logFormat = defaultLogFormat

func init() {
	if lvl, ok := os.LookupEnv(EnvLogLevel); ok {
		switch lvl {
		case "DEBUG":
			logLevel = LOG_DEBUG
		case "ERROR":
			logLevel = LOG_ERR
		default:
			logLevel = LOG_INFO
		}
	}

	if format, ok := os.LookupEnv(EnvLogFormat); ok {
		r := regexp.MustCompile(`(%s)`)
		match := r.FindAllString(format, -1)
		if match == nil || len(match) != 2 {
			fatal(ErrorArguments, "Wrong log format. Expected 2 string placeholders")
		}
		logFormat = format
	}
}

func ToLogLevel(severity int) string {
	switch severity {
	case LOG_EMERG:
		return "EMERG"
	case LOG_ALERT:
		return "ALERT"
	case LOG_CRIT:
		return "CRIT"
	case LOG_ERR:
		return "ERR"
	case LOG_WARNING:
		return "WARN"
	case LOG_NOTICE:
		return "NOTICE"
	case LOG_INFO:
		return "INFO"
	case LOG_DEBUG:
		return "DEBUG"
	}
	return ""
}
