package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"text/template"
	"time"
)

const cmdSendmail = "/usr/sbin/sendmail"
const mailTemplate = `To: {{.Notify}}
Subject: {{.Subject}}
X-Mailer: jsonmon

{{.Message}}

`

type checkFunc func(time.Duration, *regexp.Regexp) (string, string)

type Check struct {
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
	Since       string         `json:"since,omitempty" yaml:"-"`
}

func (c *Check) Run() {
	if c.Shell == "" && c.Web == "" {
		fmt.Fprintf(os.Stderr, "[%s] Ignoring entry with no either Web or shell check.\n", c.Name)
		c.Failed = true
		return
	}

	if c.Shell != "" && c.Web != "" {
		fmt.Fprintf(os.Stderr, "[%s] Web and shell checks in one block are not allowed.\n", c.Name)
		c.Failed = true
		return
	}

	// Default timeout.
	if c.Repeat == 0 {
		c.Repeat = 30
	}

	// Default to 1 attempt.
	if c.Tries == 0 {
		c.Tries = 1
	}

	// Default successful http return code
	if c.Return == 0 {
		c.Return = 200
	}

	r := time.Duration(c.Repeat) * time.Second
	s := time.Duration(c.Sleep) * time.Second

	regex, err := regexp.Compile(c.Match)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Invalid regular expression: %s\n", c.Name, err.Error())
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

			fmt.Fprintf(os.Stderr, "[%s] %s", c.Name, msg)

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

func (c *Check) runWeb(s time.Duration, r *regexp.Regexp) (subject string, msg string) {
	fmt.Printf("[%s] Running web check\n", c.Name)

	err := c.fetch(c.Web, c.Return, r)
	for i := 1; err != nil && i < c.Tries; i++ {
		time.Sleep(s)
		fmt.Printf("[%s] Running web check, retry #%d\n", c.Name, i)
		err = c.fetch(c.Web, c.Return, r)
	}

	if err != nil {
		if !c.Failed {
			c.Failed = true
			msg = err.Error()
			subject = "Failed: " + c.Name
		}
	} else {
		if c.Failed {
			c.Failed = false
			subject = "Fixed: " + c.Name
		}
	}

	return subject, msg
}

func (c *Check) runShell(s time.Duration, r *regexp.Regexp) (subject string, msg string) {
	fmt.Printf("[%s] Running shell check\n", c.Name)

	// Execute with shell with limited attempts
	out, err := c.execute(c.Shell, r)
	for i := 1; err != nil && i < c.Tries; i++ {
		time.Sleep(s)
		fmt.Printf("[%s] Running shell check, retry #%d\n", c.Name, i)
		out, err = c.execute(c.Shell, r)
	}

	if err == nil && r != nil && !r.Match(out) {
		err = errors.New("Expected:\n" + c.Match + "\n\nGot:\n" + string(out))
	}

	if err != nil {
		if !c.Failed {
			c.Failed = true
			msg = string(out) + err.Error()
			subject = "Failed: " + c.Name
		}
	} else {
		if c.Failed {
			c.Failed = false
			subject = "Fixed: " + c.Name
		}
	}

	return subject, msg
}

func (c *Check) fetch(url string, code int, r *regexp.Regexp) error {
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
		fmt.Fprintf(os.Stderr, "[%s] Error during sendmail pipe attachment: %s\n", c.Name, err.Error())
		return
	}
	err = sendmail.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Error starting cmd %s: %s\n", c.Name, cmdSendmail, err.Error())
		return
	}

	t := template.Must(template.New("mail").Parse(mailTemplate))
	err = t.Execute(stdin, map[string]string{
		"Notify":  c.Notify,
		"Subject": subject,
		"Message": message,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Error generating mail template: %s\n", c.Name, err.Error())
		return
	}

	err = stdin.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Error closing pipe: %s\n", c.Name, err.Error())
	}

	err = sendmail.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] Error sending notification: %s\n", c.Name, err.Error())
	}
}

func (c *Check) alert(msg string, failed bool) {
	var cmd string
	if msg != "" {
		cmd = fmt.Sprintf("%s %s %s %s", c.Alert, strconv.FormatBool(failed), strconv.Quote(c.Name), strconv.Quote(msg))
	} else {
		cmd = fmt.Sprintf("%s %s %s", c.Alert, strconv.FormatBool(failed), strconv.Quote(c.Name))
	}

	out, err := exec.Command(ShellPath, "-c", cmd).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %s failed\n%s\n%s\n", c.Name, c.Alert, string(out), err.Error())
	}
}
