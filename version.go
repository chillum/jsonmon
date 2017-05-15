package main

// Version is the application version.
const Version = "3.1.4"

// This one is for internal use.
var version VersionPayload

type VersionPayload struct {
	App  string `json:"jsonmon"`
	Go   string `json:"runtime"`
	Os   string `json:"os"`
	Arch string `json:"arch"`
}
