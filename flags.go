package main

import (
	"flag"
	"os"
	"path"
	"text/template"
)

const usageTemplate = `Usage:
	{{.App}} [-syslog] config.yml
	{{.App}} -version
	-----------------------------------------------
	Docs:	https://github.com/chillum/jsonmon/wiki

`

var syslogFlag, versionFlag bool

func init() {
	flag.BoolVar(&syslogFlag, "syslog", false, "-syslog")
	flag.BoolVar(&versionFlag, "version", false, "-version")

	flag.Usage = func() {
		t := template.Must(template.New("usage").Parse(usageTemplate))
		t.Execute(os.Stderr, map[string]string{
			"App": path.Base(os.Args[0]),
		})
		fatal(ErrorArguments, "missing config file parameter")
	}

	if !flag.Parsed() {
		flag.Parse()
	}
}
