package libsmtp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"path/filepath"
)

type Attachments map[string]io.Reader

type Base64Email struct {
	to   io.WriteCloser
	orig io.Writer
	buf  *bytes.Buffer
}

type splitter struct {
	to     io.Writer
	length int
}

func (s *splitter) Write(p []byte) (int, error) {
	for len(p) > s.length {
		n, err := s.to.Write(p[:s.length])
		if err != nil {
			return n, err
		}
		s.to.Write([]byte("\r\n"))
		p = p[s.length:]
	}
	n, err := s.to.Write(p)
	if err != nil {
		return n, err
	}
	n2, err := s.to.Write([]byte("\r\n"))
	return n + n2, err
}

func NewBase64Email(w io.Writer, e *base64.Encoding) *Base64Email {
	buf := bytes.NewBuffer([]byte{})
	return &Base64Email{to: base64.NewEncoder(e, buf), orig: w, buf: buf}
}

func (b Base64Email) Write(p []byte) (n int, err error) {
	return b.to.Write(p)
}

func (b *Base64Email) Close() error {
	b.to.Close()
	/* 78 is the most compatible line length for Base64'd emails */
	s := &splitter{b.orig, 78}
	io.Copy(s, b.buf)
	return nil
}

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

func SendMailWithAttachment(host string, auth *smtp.Auth, from string, to []string, msg []byte, filename string, file io.Reader) error {
	c, err := SMTPConnection(host, auth)
	if err != nil {
		return err
	}
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

func New(host string, auth *smtp.Auth, from string, to []string, msg []byte, atch Attachments) error {
	c, err := SMTPConnection(host, auth)
	if err != nil {
		return err
	}
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
	multiw := multipart.NewWriter(w)
	w.Write([]byte(fmt.Sprintf(`Content-Type: multipart/mixed; boundary="%s"`, multiw.Boundary())))
	w.Write([]byte("\r\n"))
	w.Write([]byte("--" + multiw.Boundary() + "\r\n"))
	w.Write([]byte("Content-Transfer-Encoding: quoted-printable\r\n\r\n\r\n\r\n"))
	for filename, file := range atch {
		ext := mime.TypeByExtension(filepath.Ext(filename))
		if ext == "" {
			ext = "text/plain"
		}

		h := textproto.MIMEHeader{}
		h.Add("Content-Type", ext)
		h.Add("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		h.Add("Content-Transfer-Encoding", "base64")
		newpart, err := multiw.CreatePart(h)
		if err != nil {
			return err
		}
		buf := bytes.NewBuffer([]byte{})
		bcdr := NewBase64Email(buf, base64.StdEncoding)
		_, err = io.Copy(bcdr, file)
		if err != nil {
			return err
		}
		bcdr.Close()

		_, err = io.Copy(newpart, buf)
		if err != nil {
			return err
		}
	}
	multiw.Close()
	w.Close()
	return nil
}
