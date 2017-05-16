package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"text/template"
)

const usageTemplate = `Usage:
		{{.App}} [-syslog] config.yml
		{{.App}} -version
		------------------------------------------
		Docs:	https://github.com/chillum/jsonmon/wiki
`

var syslogFlag, versionFlag bool

func init() {
	flag.BoolVar(&syslogFlag, "syslog", false, "-syslog")
	flag.BoolVar(&versionFlag, "version", false, "-version")

	flag.Usage = func() {
		t := template.Must(template.New("usage").Parse(usageTemplate))
		err := t.Execute(os.Stderr, map[string]string{
			"App": path.Base(os.Args[0]),
		})
		if err != nil {
			fmt.Println(err)
		}
		os.Exit(1)
	}

	if !flag.Parsed() {
		flag.Parse()
	}
}
