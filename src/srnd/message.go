//
// message.go
//
package srnd

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"github.com/majestrate/nacl"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"
)

type ArticleHeaders map[string][]string

func (self ArticleHeaders) Has(key string) bool {
	_, ok := self[key]
	return ok
}

func (self ArticleHeaders) Set(key, val string) {
	self[key] = []string{val}
}

func (self ArticleHeaders) Get(key, fallback string) string {
	val, ok := self[key]
	if ok {
		return val[0]
	} else {
		return fallback
	}
}

type NNTPMessage interface {
	// this message's messsge id
	MessageID() string
	// the parent message's messageid if it's specified
	Reference() string
	// the newsgroup this post is in
	Newsgroup() string
	// the name of the poster
	Name() string
	// any email address associated with the post
	Email() string
	// the subject of the post
	Subject() string
	// when this was posted
	Posted() int64
	// the path header
	Path() string
	// append something to path
	// return message with new path
	AppendPath(part string) NNTPMessage
	// the type of this message usually a mimetype
	ContentType() string
	// was this post a sage?
	Sage() bool
	// was this post a root post?
	OP() bool
	// all attachments
	Attachments() []NNTPAttachment
	// all headers
	Headers() ArticleHeaders
	// write out everything
	WriteTo(wr io.Writer) error
	// write out body
	WriteBody(wr io.Writer) error
	// attach a file
	Attach(att NNTPAttachment)
	// get the plaintext message if it exists
	Message() string
	// pack the whole message and prepare for write
	Pack()
	// get the inner nntp article that is signed and valid, returns nil if not present or invalid
	Signed() NNTPMessage
	// get the pubkey for this message if it was signed, otherwise empty string
	Pubkey() string
	// get the origin encrypted address, i2p destination or empty string for onion posters
	Addr() string
	// reset contents
	Reset()
}

type MessageReader interface {
	// read a message from a reader
	ReadMessage(r io.Reader) (NNTPMessage, error)
}

type MessageWriter interface {
	// write a message to a writer
	WriteMessage(nntp NNTPMessage, wr io.Writer) error
}

type nntpArticle struct {
	// mime header
	headers ArticleHeaders
	// multipart boundary
	boundary string
	// the text part of the message
	message NNTPAttachment
	// any attachments
	attachments []NNTPAttachment
	// the inner nntp message to be verified
	signedPart *nntpAttachment
}

func (self *nntpArticle) Reset() {
	self.headers = nil
	self.boundary = ""
	if self.message != nil {
		self.message.Reset()
		self.message = nil
	}
	if self.attachments != nil {
		for idx, _ := range self.attachments {
			self.attachments[idx].Reset()
			self.attachments[idx] = nil
		}
	}
	self.attachments = nil
	if self.signedPart != nil {
		self.signedPart.Reset()
		self.signedPart = nil
	}
}

// create a simple plaintext nntp message
func newPlaintextArticle(message, email, subject, name, instance, message_id, newsgroup string) NNTPMessage {
	nntp := &nntpArticle{
		headers: make(ArticleHeaders),
	}
	nntp.headers.Set("From", fmt.Sprintf("%s <%s>", name, email))
	nntp.headers.Set("Subject", subject)
	if isSage(subject) {
		nntp.headers.Set("X-Sage", "1")
	}
	nntp.headers.Set("Path", instance)
	nntp.headers.Set("Message-ID", message_id)
	// posted now
	nntp.headers.Set("Date", timeNowStr())
	nntp.headers.Set("Newsgroups", newsgroup)
	nntp.message = createPlaintextAttachment(message)
	nntp.Pack()
	return nntp
}

// sign an article with a seed
func signArticle(nntp NNTPMessage, seed []byte) (signed *nntpArticle, err error) {
	signed = new(nntpArticle)
	signed.headers = make(ArticleHeaders)
	h := nntp.Headers()
	// copy headers
	// copy into signed part
	for k := range h {
		if k == "Content-Type" {
			signed.headers.Set(k, "message/rfc822; charset=UTF-8")
		} else {
			v := h[k][0]
			signed.headers.Set(k, v)
		}
	}
	sha := sha512.New()
	signed.signedPart = &nntpAttachment{}
	// write body to sign buffer
	mw := io.MultiWriter(sha, signed.signedPart)
	err = nntp.WriteTo(mw)
	if err == nil {
		// build keypair
		kp := nacl.LoadSignKey(seed)
		if kp == nil {
			log.Println("failed to load seed for signing article")
			return
		}
		defer kp.Free()
		sk := kp.Secret()
		pk := getSignPubkey(sk)
		// sign it nigguh
		digest := sha.Sum(nil)
		sig := cryptoSign(digest, sk)
		// log that we signed it
		log.Printf("signed %s pubkey=%s sig=%s hash=%s", nntp.MessageID(), pk, sig, hexify(digest))
		signed.headers.Set("X-Signature-Ed25519-SHA512", sig)
		signed.headers.Set("X-PubKey-Ed25519", pk)
	}
	return
}

func (self *nntpArticle) WriteTo(wr io.Writer) (err error) {
	// write headers
	hdrs := self.headers
	for hdr, hdr_vals := range hdrs {
		for _, hdr_val := range hdr_vals {
			wr.Write([]byte(hdr))
			wr.Write([]byte(": "))
			wr.Write([]byte(hdr_val))
			_, err = wr.Write([]byte{10})
			if err != nil {
				log.Println("error while writing headers", err)
				return
			}
		}
	}
	// done headers
	_, err = wr.Write([]byte{10})
	if err != nil {
		log.Println("error while writing body", err)
		return
	}

	// write body
	err = self.WriteBody(wr)
	return
}

func (self *nntpArticle) Pubkey() string {
	return self.headers.Get("X-PubKey-Ed25519", self.headers.Get("X-Pubkey-Ed25519", ""))
}

func (self *nntpArticle) Signed() NNTPMessage {
	if self.signedPart != nil {
		buff := new(bytes.Buffer)
		buff.Write(self.signedPart.body)
		msg, err := read_message(buff)
		buff.Reset()
		if err == nil {
			return msg
		}
		log.Println("failed to load signed part", err)
	}
	return nil
}

func (self *nntpArticle) MessageID() (msgid string) {
	for _, h := range []string{"Message-ID", "Messageid", "MessageID", "Message-Id"} {
		mid := self.headers.Get(h, "")
		if mid != "" {
			msgid = string(mid)
			return
		}
	}
	return
}

func (self *nntpArticle) Pack() {
	if len(self.attachments) > 0 {
		if len(self.boundary) == 0 {
			// we have no boundry, set it
			self.boundary = randStr(24)
			// set headers
			self.headers.Set("Mime-Version", "1.0")
			self.headers.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", self.boundary))
		}
	} else if self.signedPart == nil {
		self.headers.Set("Content-Type", "text/plain; charset=utf-8")
	}
}

func (self *nntpArticle) Reference() string {
	return self.headers.Get("Reference", self.headers.Get("References", ""))
}

func (self *nntpArticle) Newsgroup() string {
	return self.headers.Get("Newsgroups", "")
}

func (self *nntpArticle) Name() string {
	from := self.headers.Get("From", "anonymous <a@no.n>")
	idx := strings.Index(from, "<")
	if idx > 1 {
		return from[:idx]
	}
	return "[Invalid From header]"
}

func (self *nntpArticle) Addr() (addr string) {
	addr = self.headers.Get("X-Encrypted-Ip", "")
	if addr != "" {
		return
	}

	addr = self.headers.Get("X-Encrypted-IP", "")
	if addr != "" {
		return
	}

	addr = self.headers.Get("X-I2P-DestHash", "")
	if addr != "" {
		if addr == "None" {
			return ""
		}
		return
	}

	addr = self.headers.Get("X-I2p-Desthash", "")
	return
}

func (self *nntpArticle) Email() string {
	from := self.headers.Get("From", "anonymous <a@no.n>")
	idx := strings.Index(from, "<")
	if idx > 2 {
		return from[:idx-2]
	}
	return "[Invalid From header]"

}

func (self *nntpArticle) Subject() string {
	return self.headers.Get("Subject", "")
}

func (self *nntpArticle) Posted() int64 {
	posted := self.headers.Get("Date", "")
	t, err := time.Parse(time.RFC1123Z, posted)
	if err == nil {
		return t.Unix()
	}
	return 0
}

func (self *nntpArticle) Message() string {
	return strings.Trim(self.message.AsString(), "\x00")
}

func (self *nntpArticle) Path() string {
	return self.headers.Get("Path", "unspecified")
}

func (self *nntpArticle) Headers() ArticleHeaders {
	return self.headers
}

func (self *nntpArticle) AppendPath(part string) NNTPMessage {
	if self.headers.Has("Path") {
		self.headers.Set("Path", part+"!"+self.Path())
	} else {
		self.headers.Set("Path", part)
	}
	return self
}
func (self *nntpArticle) ContentType() string {
	// assumes text/plain if unspecified
	return self.headers.Get("Content-Type", "text/plain; charset=UTF-8")
}

func (self *nntpArticle) Sage() bool {
	return self.headers.Get("X-Sage", "") == "1"
}

func (self *nntpArticle) OP() bool {
	return self.headers.Get("Reference", self.headers.Get("References", "")) == ""
}

func (self *nntpArticle) Attachments() []NNTPAttachment {
	return self.attachments
}

func (self *nntpArticle) Attach(att NNTPAttachment) {
	self.attachments = append(self.attachments, att)
}

func (self *nntpArticle) WriteBody(wr io.Writer) (err error) {
	// this is a signed message, don't treat it special
	if self.signedPart != nil {
		_, err = self.signedPart.WriteTo(wr)
		return
	}
	if len(self.attachments) == 0 {
		// write plaintext and be done
		_, err = self.message.WriteTo(wr)
		return
	}
	content_type := self.ContentType()
	_, params, err := mime.ParseMediaType(content_type)
	if err != nil {
		log.Println("failed to parse media type", err)
		return err
	}

	boundary, ok := params["boundary"]
	if ok {
		w := multipart.NewWriter(wr)

		err = w.SetBoundary(boundary)
		if err == nil {
			attachments := []NNTPAttachment{self.message}
			attachments = append(attachments, self.attachments...)
			for _, att := range attachments {
				hdr := att.Header()
				if hdr == nil {
					hdr = make(textproto.MIMEHeader)
				}
				hdr.Set("Content-Transfer-Encoding", "base64")
				part, err := w.CreatePart(hdr)
				str := att.Filedata()
				att = nil
				dat := []byte(str)
				_, err = part.Write(dat)
				str = ""
				dat = nil
				if err != nil {
					break
				}
				part = nil
			}
		}
		if err != nil {
			log.Println("error writing part", err)
		}
		err = w.Close()
		w = nil
	} else {
		_, err = self.message.WriteTo(wr)
	}
	return err
}
