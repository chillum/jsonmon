package main

import "runtime"

// Version is the application version.
const Version = "3.1.4"

type VersionPayload struct {
	App  string `json:"jsonmon"`
	Go   string `json:"runtime"`
	Os   string `json:"os"`
	Arch string `json:"arch"`
}

func NewVersionPayload() *VersionPayload {
	return &VersionPayload{
		App:  Version,
		Go:   runtime.Version(),
		Os:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
}
