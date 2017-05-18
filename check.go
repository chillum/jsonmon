package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"text/template"
	"time"
)

const (
	cmdSendmail     = "/usr/sbin/sendmail"
	defaultTries    = 1
	defaultRepeat   = 30 // sec
	defaultHttpCode = http.StatusOK
)

const mailTemplate = `To: {{.Notify}}
Subject: {{.Subject}}
X-Mailer: {{.AppName}}

{{.Message}}

`

type checkFunc func(time.Duration, *regexp.Regexp) (string, string)

type Check struct {
	m           sync.RWMutex
	Name        string         `json:"name,omitempty"`
	Web         string         `json:"web,omitempty"`
	Shell       string         `json:"shell,omitempty"`
	Match       string         `json:"-"`
	MatchRegexp *regexp.Regexp `json:"-" yaml:"-"`
	Return      int            `json:"-"`
	Notify      string         `json:"-"`
	Alert       string         `json:"-"`
	Tries       int            `json:"-"`
	Repeat      int            `json:"-"`
	Sleep       int            `json:"-"`
	Failed      bool           `json:"failed" yaml:"-"`
	Error       string         `json:"error" yaml:"-"`
	Since       string         `json:"since,omitempty" yaml:"-"`
}

type Checks []*Check

func (c Checks) RLock() {
	for _, check := range c {
		check.m.RLock()
	}
}

func (c Checks) RUnlock() {
	for _, check := range c {
		check.m.RUnlock()
	}
}

func (c *Check) Run() {
	if c.Shell == "" && c.Web == "" {
		logs.Log(LOG_WARNING, fmt.Sprintf("[%s] Ignoring entry with no either Web or shell check.", c.Name))
		c.Failed = true
		return
	}

	if c.Shell != "" && c.Web != "" {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Web and shell checks in one block are not allowed.", c.Name))
		c.Failed = true
		return
	}

	// Default timeout.
	if c.Repeat == 0 {
		c.Repeat = defaultRepeat
	}

	// Default to 1 attempt.
	if c.Tries == 0 {
		c.Tries = defaultTries
	}

	// Default successful http return code
	if c.Return == 0 {
		c.Return = defaultHttpCode
	}

	r := time.Duration(c.Repeat) * time.Second
	s := time.Duration(c.Sleep) * time.Second

	regex, err := regexp.Compile(c.Match)
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Invalid regular expression: %s", c.Name, err.Error()))
		return
	}

	var check checkFunc
	if c.Web != "" {
		if c.Name == "" {
			c.Name = c.Web
		}
		check = c.runWeb
	} else {
		if c.Name == "" {
			c.Name = c.Shell
		}
		check = c.runShell
	}

	for {
		subject, msg := check(s, regex)

		if subject != "" {
			c.Since = time.Now().Format(time.RFC3339)
			logs.Log(LOG_ERR, fmt.Sprintf("[%s] %s", c.Name, msg))
			if c.Notify != "" {
				go c.notify(subject, msg)
			}
			if c.Alert != "" {
				go c.alert(msg, c.Failed)
			}
		}

		time.Sleep(r)
	}
}

// Updates the check status concurrency safe to failed.
// If there was no status change the len of the returned subject is zero.
func (c *Check) MarkFailed(errMsg string) (subject string) {
	c.m.Lock()
	if !c.Failed {
		c.Failed = true
		c.Error = errMsg
		subject = "Failed: " + c.Name
	}
	c.m.Unlock()
	return subject
}

// Updates the check status concurrency safe to healthy.
// If there was no status change the len of the returned subject is zero.
func (c *Check) MarkHealthy() (subject string) {
	c.m.Lock()
	if c.Failed {
		c.Failed = false
		c.Error = ""
		subject = "Fixed: " + c.Name
	}
	c.m.Unlock()
	return subject
}

func (c *Check) runWeb(s time.Duration, r *regexp.Regexp) (subject string, msg string) {
	logs.Log(LOG_INFO, fmt.Sprintf("[%s] Running web check", c.Name))

	err := c.fetch(c.Web, c.Return, r)
	for i := 1; err != nil && i < c.Tries; i++ {
		time.Sleep(s)
		logs.Log(LOG_DEBUG, fmt.Sprintf("[%s] Running web check, retry #%2d\n", c.Name, i))
		err = c.fetch(c.Web, c.Return, r)
	}

	if err != nil {
		msg = err.Error()
		subject = c.MarkFailed(msg)
	} else {
		subject = c.MarkHealthy()
	}

	return subject, msg
}

func (c *Check) runShell(s time.Duration, r *regexp.Regexp) (subject string, msg string) {
	logs.Log(LOG_INFO, fmt.Sprintf("[%s] Running shell check", c.Name))
	// Execute with shell with limited attempts
	out, err := c.execute(c.Shell, r)
	for i := 1; err != nil && i < c.Tries; i++ {
		time.Sleep(s)
		logs.Log(LOG_INFO, fmt.Sprintf("[%s] Running shell check, retry #%2d", c.Name, i))
		out, err = c.execute(c.Shell, r)
	}

	if err == nil && r != nil && !r.Match(out) {
		err = errors.New("Expected:\n" + c.Match + "\n\nGot:\n" + string(out))
	}

	if err != nil {
		msg = string(out) + err.Error()
		subject = c.MarkFailed(msg)
	} else {
		subject = c.MarkHealthy()
	}

	return subject, msg
}

func (c *Check) fetch(url string, code int, r *regexp.Regexp) error {
	logs.Log(LOG_DEBUG, fmt.Sprintf("[%s] Fetching url: %q", c.Name, url))
	resp, err := http.Get(url)

	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
	}()

	if err != nil {
		return err
	}

	// Check status code.
	if resp.StatusCode != code {
		return errors.New(url + " returned " + strconv.Itoa(resp.StatusCode))
	}

	if resp == nil || c.Match == "" {
		// No regexp check possible or required.
		return nil
	}

	if c.MatchRegexp == nil {
		c.MatchRegexp, err = regexp.Compile(c.Match)
		if err != nil {
			return errors.New(fmt.Sprintf("Cannot compile regexp: %s", c.Match))
		}
	}

	// Match regexp.
	var body []byte
	body, _ = ioutil.ReadAll(resp.Body)
	if !c.MatchRegexp.Match(body) {
		return errors.New("Expected:\n" + c.Match + "\n\nReceived:\n" + string(body))
	}

	// Everything was just fine
	return nil
}

func (c *Check) execute(cmd string, r *regexp.Regexp) (out []byte, err error) {
	logs.Log(LOG_DEBUG, fmt.Sprintf("[%s] Executing cmd %q", c.Name, cmd))
	out, err = exec.Command(ShellPath, "-c", cmd).CombinedOutput()
	if err == nil && r != nil && !r.Match(out) {
		err = errors.New("Expected:\n" + c.Match + "\n\nGot:\n" + string(out))
	}
	return out, err
}

func (c *Check) notify(subject string, message string) {
	sendmail := exec.Command(cmdSendmail, "-t")
	stdin, err := sendmail.StdinPipe()
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Error during sendmail pipe attachment: %s", c.Name, err.Error()))
		return
	}
	err = sendmail.Start()
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Error starting cmd %s: %s", c.Name, cmdSendmail, err.Error()))
		return
	}

	t := template.Must(template.New("mail").Parse(mailTemplate))
	err = t.Execute(stdin, map[string]string{
		"AppName": AppName,
		"Notify":  c.Notify,
		"Subject": subject,
		"Message": message,
	})
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Error generating mail template: %s", c.Name, err.Error()))
		return
	}

	err = stdin.Close()
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Error closing pipe: %s", c.Name, err.Error()))
	}

	err = sendmail.Wait()
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] Error sending notification: %s", c.Name, err.Error()))
	}
}

func (c *Check) alert(msg string, failed bool) {
	logs.Log(LOG_DEBUG, fmt.Sprintf("[%s] Sending alert", c.Name))
	var cmd string
	if msg != "" {
		cmd = fmt.Sprintf("%s %s %s %s", c.Alert, strconv.FormatBool(failed), strconv.Quote(c.Name), strconv.Quote(msg))
	} else {
		cmd = fmt.Sprintf("%s %s %s", c.Alert, strconv.FormatBool(failed), strconv.Quote(c.Name))
	}

	out, err := exec.Command(ShellPath, "-c", cmd).CombinedOutput()
	if err != nil {
		logs.Log(LOG_ERR, fmt.Sprintf("[%s] %s failed:\n\t%s\n\t%s", c.Name, c.Alert, string(out), err.Error()))
	}
}
