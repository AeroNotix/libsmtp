package libsmtp

import (
	"fmt"
	"io"
	"mime"
	"net/smtp"
	"path/filepath"
)

func SMTPConnection(host string, auth *smtp.Auth) (*smtp.Client, error) {
	c, err := smtp.Dial(host)
	if err != nil {
		return nil, err
	}

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err = c.StartTLS(nil); err != nil {
			return nil, err
		}
	}

	if err = c.Auth(*auth); err != nil {
		return nil, err
	}

	return c, nil
}

func SendMailWithAttachment(c *smtp.Client, from string, to []string, msg []byte, filename string, file io.Reader) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	ext := mime.TypeByExtension(filepath.Ext(filename))
	if ext == "" {
		ext = "text/plain"
	}
	w.Write([]byte(fmt.Sprintf(`Content-type: %s; name="%s"`, ext, filename)))
	w.Write([]byte("\n"))
	w.Write([]byte(fmt.Sprintf(`Content-Disposition: attachment; filename="%s"`, filename)))
	w.Write([]byte("\n\n\n"))
	io.Copy(w, file)
	err = w.Close()
	if err != nil {
		return err
	}
	return nil
}
