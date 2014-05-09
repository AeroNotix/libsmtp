package libsmtp

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"strings"
)

const CRLF = "\r\n"

type Attachments map[string]io.Reader

// Base64Email is a wrapper around the encoding/base64 encoder since
// e-mails require a little more formatting when pieces are encoded,
// rather than a single block of base64'd data we need to split it up
// into fixed-sized blocks.
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
		s.to.Write([]byte(CRLF))
		p = p[s.length:]
	}
	n, err := s.to.Write(append(p, []byte(CRLF)...))
	return n, err
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
		server, _, err := net.SplitHostPort(host)
		if err != nil {
			return nil, err
		}
		if err = c.StartTLS(&tls.Config{ServerName: server}); err != nil {
			return nil, err
		}
	}
	if err = c.Auth(*auth); err != nil {
		return nil, err
	}

	return c, nil
}

func SendMailWithAttachments(host string, auth *smtp.Auth, from, subject string, to []string, msg []byte, atch Attachments) error {
	c, err := SMTPConnection(host, auth)
	if err != nil {
		return err
	}
	defer c.Quit()
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
	err = write(
		w,
		fmt.Sprintf("From: %s%s", from, CRLF),
		fmt.Sprintf("Subject: %s%s", subject, CRLF),
		fmt.Sprintf("To: %s%s", strings.Join(to, ","), CRLF),
	)
	if err != nil {
		return err
	}
	if atch != nil {
		err = write(
			w,
			fmt.Sprintf(`Content-Type: multipart/mixed; boundary="%s"%s`, multiw.Boundary(), CRLF),
			"--"+multiw.Boundary()+CRLF,
			"Content-Transfer-Encoding: quoted-printable",
		)
	} else {
		return write(w, strings.Repeat(CRLF, 4), string(msg), strings.Repeat(CRLF, 4))
	}
	// We write either the message, or 4*CRLF since SMTP supports files
	// being sent without an actual body.
	if msg != nil {
		err = write(w,
			fmt.Sprintf(
				"%s%s%s",
				strings.Repeat(CRLF, 2),
				msg,
				strings.Repeat(CRLF, 2),
			),
		)
		if err != nil {
			return err
		}
	} else {
		if err := write(w, strings.Repeat(CRLF, 4)); err != nil {
			return err
		}
	}
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
		if _, err = io.Copy(bcdr, file); err != nil {
			return err
		}
		if err = bcdr.Close(); err != nil {
			return err
		}
		if _, err = io.Copy(newpart, buf); err != nil {
			return err
		}
	}
	if err = multiw.Close(); err != nil {
		return err
	}
	return w.Close()
}

// Helper method to make writing to an io.Writer over and over nicer.
func write(w io.Writer, data ...string) error {
	for _, part := range data {
		_, err := w.Write([]byte(part))
		if err != nil {
			return err
		}
	}
	return nil
}
