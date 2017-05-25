package main

import (
	"bytes"
	"io"
	"os/exec"
	"strconv"
)

// Mail notifications.
func notify(to *string, subject *string, message *string) {
	// Make the message.
	var msg bytes.Buffer
	var err error
	msg.WriteString("To: ")
	msg.WriteString(*to)
	msg.WriteString("\nSubject: ")
	msg.WriteString(*subject)
	msg.WriteString("\nX-Mailer: jsonmon\n\n")
	if message != nil {
		msg.WriteString(*message)
	}
	msg.WriteString("\n.\n")
	// And send it.
	sendmail := exec.Command("/usr/sbin/sendmail", "-t")
	stdin, _ := sendmail.StdinPipe()
	err = sendmail.Start()
	if err != nil {
		log(3, err.Error())
	}
	io.WriteString(stdin, msg.String())
	sendmail.Wait()
}

// Executes callback. Passes args: true/false, check's name, message.
func alert(cmd *string, name *string, msg *string, failed bool) {
	var out []byte
	var err error
	if msg != nil {
		out, err = exec.Command(*cmd, strconv.FormatBool(failed), *name, *msg).CombinedOutput()
	} else {
		out, err = exec.Command(*cmd, strconv.FormatBool(failed), *name).CombinedOutput()
	}
	if err != nil {
		log(3, *cmd+" failed\n"+string(out)+err.Error())
	}
}
