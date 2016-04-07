//
// attachment.go -- nntp attachements
//

package srnd

import (
	"crypto/sha512"
	"encoding/base32"
	"encoding/base64"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

type NNTPAttachment interface {
	io.WriterTo
	io.Writer

	// the name of the file
	Filename() string
	// the filepath to the saved file
	Filepath() string
	// the mime type of the attachment
	Mime() string
	// the file extension of the attachment
	Extension() string
	// get the sha512 hash of the attachment
	Hash() []byte
	// do we need to generate a thumbnail?
	NeedsThumbnail() bool
	// mime header
	Header() textproto.MIMEHeader
	// make into a model
	ToModel(prefix string) AttachmentModel
	// base64'd file data
	Filedata() string
	// as raw string
	AsString() string
	// reset contents
	Reset()
}

type nntpAttachment struct {
	ext      string
	mime     string
	filename string
	filepath string
	hash     []byte
	header   textproto.MIMEHeader
	body     []byte
}

func (self *nntpAttachment) Reset() {
	self.body = nil
	self.header = nil
	self.hash = nil
	self.filepath = ""
	self.filename = ""
	self.mime = ""
	self.ext = ""
}

func (self *nntpAttachment) ToModel(prefix string) AttachmentModel {
	return &attachment{
		prefix: prefix,
		Path:   self.Filepath(),
		Name:   self.Filename(),
	}
}

func (self *nntpAttachment) Write(b []byte) (int, error) {
	self.body = append(self.body, b...)
	return len(b), nil
}

func (self *nntpAttachment) AsString() string {
	if self.body == nil {
		return ""
	}
	return string(self.body)
}

func (self *nntpAttachment) Filedata() string {
	e := base64.StdEncoding
	str := e.EncodeToString(self.body)
	e = nil
	return str
}

func (self *nntpAttachment) Filename() string {
	return self.filename
}

func (self *nntpAttachment) Filepath() string {
	return self.filepath
}

func (self *nntpAttachment) Mime() string {
	return self.mime
}

func (self *nntpAttachment) Extension() string {
	return self.ext
}

func (self *nntpAttachment) WriteTo(wr io.Writer) (int64, error) {
	w, err := wr.Write(self.body)
	return int64(w), err
}

func (self *nntpAttachment) Hash() []byte {
	// hash it if we haven't already
	if self.hash == nil || len(self.hash) == 0 {
		h := sha512.Sum512(self.body)
		self.hash = h[:]
	}
	return self.hash
}

// TODO: detect
func (self *nntpAttachment) NeedsThumbnail() bool {
	for _, ext := range []string{".png", ".jpeg", ".jpg", ".gif", ".bmp", ".webm", ".mp4", ".avi", ".mpeg", ".mpg", ".ogg", ".mp3", ".oga", ".opus", ".flac", ".ico"} {
		if ext == strings.ToLower(self.ext) {
			return true
		}
	}
	return false
}

func (self *nntpAttachment) Header() textproto.MIMEHeader {
	return self.header
}

type AttachmentSaver interface {
	// save an attachment given its original filename
	// pass in a reader that reads the content of the attachment
	Save(filename string, r io.Reader) error
}

// create a plaintext attachment
func createPlaintextAttachment(msg string) NNTPAttachment {
	header := make(textproto.MIMEHeader)
	mime := "text/plain; charset=UTF-8"
	header.Set("Content-Type", mime)
	return &nntpAttachment{
		mime:   mime,
		ext:    ".txt",
		body:   []byte(msg),
		header: header,
	}
}

// assumes base64'd
func createAttachment(content_type, fname string, body io.Reader) NNTPAttachment {

	media_type, _, err := mime.ParseMediaType(content_type)
	if err == nil {
		a := new(nntpAttachment)
		dec := base64.NewDecoder(base64.StdEncoding, body)
		var b [1024]byte
		for {
			var n int
			n, err = dec.Read(b[:])
			if err != nil {
				if err == io.EOF {
					err = nil
				}
				break
			}
			a.body = append(a.body, b[:n]...)
		}
		if err == nil {
			a.header = make(textproto.MIMEHeader)
			a.mime = media_type + "; charset=UTF-8"
			idx := strings.LastIndex(fname, ".")
			a.ext = ".txt"
			if idx > 0 {
				a.ext = fname[idx:]
			}
			a.header.Set("Content-Disposition", `form-data; filename="`+fname+`"; name="attachment"`)
			a.header.Set("Content-Type", a.mime)
			a.header.Set("Content-Transfer-Encoding", "base64")
			h := sha512.Sum512(a.body)
			hashstr := base32.StdEncoding.EncodeToString(h[:])
			a.hash = h[:]
			a.filepath = hashstr + a.ext
			a.filename = fname
			return a
		}
	}
	return nil
}

func readAttachmentFromMimePart(part *multipart.Part) NNTPAttachment {
	hdr := part.Header
	var body []byte
	content_type := hdr.Get("Content-Type")
	media_type, _, err := mime.ParseMediaType(content_type)
	fname := part.FileName()
	idx := strings.LastIndex(fname, ".")
	ext := ".txt"
	if idx > 0 {
		ext = fname[idx:]
	}

	transfer_encoding := hdr.Get("Content-Transfer-Encoding")
	var r io.Reader
	if transfer_encoding == "base64" {
		// decode
		r = base64.NewDecoder(base64.StdEncoding, part)
	} else {
		r = part
	}
	var buff [1024]byte
	for {
		var n int
		n, err = r.Read(buff[:])
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			break
		}
		body = append(body, buff[:n]...)
	}
	// clear reference
	part = nil
	if err != nil {
		log.Println("failed to read attachment from mimepart", err)
		return nil
	}
	sha := sha512.Sum512(body)
	enc := base32.StdEncoding
	hashstr := enc.EncodeToString(sha[:])
	fpath := hashstr + ext
	return &nntpAttachment{
		body:     body,
		header:   hdr,
		mime:     media_type,
		filename: fname,
		filepath: fpath,
		ext:      ext,
		hash:     sha[:],
	}
}
