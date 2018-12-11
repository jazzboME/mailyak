package mailyak

import (
	"bytes"
	"fmt"
	"net/smtp"
	"regexp"
	"strings"
	"time"
)

// TODO: in the future, when aliasing is supported or we're making a breaking
// API change anyway, change the MailYak struct name to Email.

// MailYak represents an email.
type MailYak struct {
	html  BodyPart
	plain BodyPart

	toAddrs        []string
	ccAddrs        []string
	bccAddrs       []string
	subject        string
	fromAddr       string
	fromName       string
	replyTo        string
	headers        map[string]string // arbitrary headers
	attachments    []attachment
	auth           smtp.Auth
	trimRegex      *regexp.Regexp
	host           string
	writeBccHeader bool
	date           string
}

// New returns an instance of MailYak using host as the SMTP server, and
// authenticating with auth where required.
//
// host must include the port number (i.e. "smtp.itsallbroken.com:25")
//
// 		mail := mailyak.New("smtp.itsallbroken.com:25", smtp.PlainAuth(
// 			"",
// 			"username",
// 			"password",
// 			"stmp.itsallbroken.com",
//		))
//
func New(host string, auth smtp.Auth) *MailYak {
	return &MailYak{
		headers:        map[string]string{},
		host:           host,
		auth:           auth,
		trimRegex:      regexp.MustCompile("\r?\n"),
		writeBccHeader: false,
		date:           time.Now().Format(time.RFC1123Z),
	}
}

// Send attempts to send the built email via the configured SMTP server.
//
// Attachments are read when Send() is called, and any connection/authentication
// errors will be returned by Send().
func (m *MailYak) Send() error {
	buf, err := m.buildMime()
	if err != nil {
		return err
	}

	return smtp.SendMail(
		m.host,
		m.auth,
		m.fromAddr,
		append(append(m.toAddrs, m.ccAddrs...), m.bccAddrs...),
		buf.Bytes(),
	)
}

// SendMail attempts to provide a more featureful version of the Send command
//
// Goal is to allow low level adjustments, such as setting the HELO response
func (m *MailYak) SendMail(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	if err := validateLine(from); err != nil {
		return err
	}
	for _, recp := range to {
		if err := validateLine(recp); err != nil {
			return err
		}
	}
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello("UMF-FW1.um.maine.edu"); err != nil {
		return err
	}
	
	//if err = c.hello(); err != nil {
	//	return err
	//}

	if ok, _ := c.Extension("STARTTLS"); ok {
		config := &tls.Config{ServerName: c.serverName}
		if testHookStartTLS != nil {
			testHookStartTLS(config)
		}
		if err = c.StartTLS(config); err != nil {
			return err
		}
	}
	if a != nil && c.ext != nil {
		if _, ok := c.ext["AUTH"]; !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		if err = c.Auth(a); err != nil {
			return err
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

// MimeBuf returns the buffer containing all the RAW MIME data.
//
// MimeBuf is typically used with an API service such as Amazon SES that does
// not use an SMTP interface.
func (m *MailYak) MimeBuf() (*bytes.Buffer, error) {
	buf, err := m.buildMime()
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// String returns a redacted description of the email state, typically for
// logging or debugging purposes.
//
// Authentication information is not included in the returned string.
func (m *MailYak) String() string {
	var (
		att    []string
		custom string
	)
	for _, a := range m.attachments {
		att = append(att, "{filename: "+a.filename+"}")
	}

	if len(m.headers) > 0 {
		var hdrs []string
		for k, v := range m.headers {
			hdrs = append(hdrs, fmt.Sprintf("%s: %q", k, v))
		}
		custom = strings.Join(hdrs, ", ") + ", "
	}
	return fmt.Sprintf(
		"&MailYak{date: %q, from: %q, fromName: %q, html: %v bytes, plain: %v bytes, toAddrs: %v, "+
			"bccAddrs: %v, subject: %q, %vhost: %q, attachments (%v): %v, auth set: %v}",
		m.date,
		m.fromAddr,
		m.fromName,
		len(m.HTML().String()),
		len(m.Plain().String()),
		m.toAddrs,
		m.bccAddrs,
		m.subject,
		custom,
		m.host,
		len(att),
		att,
		m.auth != nil,
	)
}

// HTML returns a BodyPart for the HTML email body.
func (m *MailYak) HTML() *BodyPart {
	return &m.html
}

// Plain returns a BodyPart for the plain-text email body.
func (m *MailYak) Plain() *BodyPart {
	return &m.plain
}

// validateLine checks to see if a line has CR or LF as per RFC 5321
func validateLine(line string) error {
	if strings.ContainsAny(line, "\n\r") {
		return errors.New("smtp: A line must not contain CR or LF")
	}
	return nil
}
