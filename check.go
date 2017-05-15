package main

// Check details.
type Check struct {
	Name   string `json:"name,omitempty"`
	Web    string `json:"web,omitempty"`
	Shell  string `json:"shell,omitempty"`
	Match  string `json:"-"`
	Return int    `json:"-"`
	Notify string `json:"-"`
	Alert  string `json:"-"`
	Tries  int    `json:"-"`
	Repeat int    `json:"-"`
	Sleep  int    `json:"-"`
	Failed bool   `json:"failed" yaml:"-"`
	Since  string `json:"since,omitempty" yaml:"-"`
}
