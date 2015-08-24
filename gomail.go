package gomail

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"os"
	"path"
	"strings"
)

type Email struct {
	data        io.WriteCloser
	alternative *multipart.Writer
	html        *multipart.Writer
}

// New constructs a new Email for sending.
func New(server string, from, to string, subject string) (*Email, error) {
	if "" == server {
		server = "localhost:25"
	}
	msg, err := smtp.Dial(server)
	if nil != err {
		return nil, err
	}
	msg.Mail(from)
	msg.Rcpt(to)
	data, err := msg.Data()

	if nil != err {
		return nil, err
	}

	m := multipart.NewWriter(data)

	fmt.Fprintf(data, "To: <%v>\n", to)
	fmt.Fprintf(data, "Subject: %v\n", subject)
	fmt.Fprintf(data, "MIME-Version: 1.0\n")
	fmt.Fprintf(data, `Content-Type: multipart/alternative; boundary="%v"%v`, m.Boundary(), "\n")
	fmt.Fprintf(data, "\n")
	fmt.Fprintf(data, "This is a multi-part message in MIME format.\n")

	email := &Email{
		data:        data,
		alternative: m,
	}
	return email, nil
}

// Send sends the emial, returning an error if one occurs.
func (e *Email) Send() error {
	var err error
	if nil != e.html {
		err = e.html.Close()
		if nil != err {
			return err
		}
	}
	if nil != e.alternative {
		err = e.alternative.Close()
		if nil != err {
			return err
		}
	}
	return e.data.Close()
}

// Text sets the text content of the email.
func (e *Email) Text(text string) {
	out, _ := e.alternative.CreatePart(textproto.MIMEHeader(map[string][]string{
		"Content-Type": []string{"text/plain; charset=utf-8"},
	}))
	fmt.Fprintln(out, text)
}

// Html sets the HTML content of the email. You CANNOT
// call Html twice - it will panic.
func (e *Email) Html(html string) *Email {
	if nil != e.html {
		panic("Cannot call Html twice on Email")
	}
	// We have a problem around creating these two, since the multipart.NewWriter requires the writer
	// for the containing Part, and the containing Part needs the multipart's boundary in it's creation.
	// So we create a Pipe that the inner multipart Writer will write to.
	// Then we can create the containing Part writinig to the Pipe's Writer
	// The we launch a goroutine to copy from the Pipe's Reader to the containing Part's Writer.
	bndIn, bndOut := io.Pipe()
	e.html = multipart.NewWriter(bndOut)
	// every write on bndOut becomes a 'read' on bndIn
	htmlOut, _ := e.alternative.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{fmt.Sprintf("multipart/related; boundary=\"%v\"", e.html.Boundary())},
	})
	go io.Copy(htmlOut, bndIn)

	inner, _ := e.html.CreatePart(textproto.MIMEHeader{
		"Content-Type": []string{"text/html"},
	})
	// Actually copy the HTML to the content
	fmt.Fprintln(inner, html)
	return e
}

// InlineFile takes the provided file and inlines it into the
// email. You MUST call Html(..) first, or you will get a panic.
func (e *Email) InlineFile(filename string) (*Email, error) {
	if nil == e.html {
		panic("You must call Html(..) before you can call InlineFile")
	}
	name := path.Base(filename)

	mimeType := "image/jpg"
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case "png":
		mimeType = "image/png"
	case "gif":
		mimeType = "image/gif"
	case "jpg":
		mimeType = "image/jpeg"
	}
	inner, _ := e.html.CreatePart(textproto.MIMEHeader{
		"Content-Type":              []string{fmt.Sprintf(`%v; name="%v"`, mimeType, name)},
		"Content-ID":                []string{fmt.Sprintf("<%v>", name)},
		"Content-Disposition":       []string{fmt.Sprintf(`inline; filename="%v"`, name)},
		"Content-Transfer-Encoding": []string{"base64"},
	})

	b64 := base64.NewEncoder(base64.StdEncoding, inner)
	fileIn, err := os.Open(filename)
	if nil != err {
		return nil, err
	}
	defer fileIn.Close()
	_, err = io.Copy(b64, fileIn)
	if nil != err {
		return nil, err
	}
	b64.Close()
	return e, nil
}
