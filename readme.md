libsmtp is an add-on package for Go's net/smtp package.

The original net/smtp is rightfully incomplete, Go intends the
standard library to remain small, nimble and manageable. The SMTP
specification is none of those things.

libsmtp provides support for attachments and filling the client-fields
in e-mail messages.

```go

package main

import (
	"fmt"
	"github.com/AeroNotix/libsmtp"
	"net/smtp"
	"io"
	"os"
)

func main() {
	auth := smtp.PlainAuth(
		"",
		"your_email",
		"password",
		"smtp.domain.com",
	)
	f, _ := os.Open("/path/to/file/")
	fmt.Println(
		libsmtp.SendMailWithAttachments(
			"smtp.domain.com:port",
			&auth,
			"from@address.com",
			"Y u no talk?",
			[]string{"recipient@address.com"},
			[]byte("Message!"),
			map[string]io.Reader{
				"filename": f,
			},
		),
	)

}
```
