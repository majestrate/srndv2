//
// store.go
//

package srnd

import (
	"bufio"
	"compress/gzip"
	"crypto/sha512"
	"errors"
	"github.com/majestrate/nacl"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ArticleStore interface {
	MessageReader

	// get the filepath for an attachment
	AttachmentFilepath(fname string) string
	// get the filepath for an attachment's thumbnail
	ThumbnailFilepath(fname string) string
	// do we have this article?
	HasArticle(msgid string) bool
	// create a file for a message
	CreateFile(msgid string) io.WriteCloser
	// create a file for a temp message, returns nil if it's already open
	CreateTempFile(msgid string) io.WriteCloser
	// get the filename of a message
	GetFilename(msgid string) string
	// get the filename of a temp message
	GetTempFilename(msgid string) string
	// Get a message given its messageid
	GetMessage(msgid string) NNTPMessage
	// open a message in the store for reading given its message-id
	// return io.ReadCloser, error
	OpenMessage(msgid string) (io.ReadCloser, error)
	// get a temp message given its messageid
	// temp message is deleted once read
	ReadTempMessage(msgid string) NNTPMessage
	// store a post
	StorePost(nntp NNTPMessage) error
	// get article headers only
	GetHeaders(msgid string) ArticleHeaders
	// get our temp directory for articles
	TempDir() string
	// get a list of all the attachments we have
	GetAllAttachments() ([]string, error)
	// generate a thumbnail
	GenerateThumbnail(fname string) error
	// did we enable compression?
	Compression() bool
}
type articleStore struct {
	directory    string
	temp         string
	attachments  string
	thumbs       string
	database     Database
	convert_path string
	ffmpeg_path  string
	sox_path     string
	compression  bool
	compWriter   *gzip.Writer
}

func createArticleStore(config map[string]string, database Database) ArticleStore {
	store := &articleStore{
		directory:    config["store_dir"],
		temp:         config["incoming_dir"],
		attachments:  config["attachments_dir"],
		thumbs:       config["thumbs_dir"],
		convert_path: config["convert_bin"],
		ffmpeg_path:  config["ffmpegthumbnailer_bin"],
		sox_path:     config["sox_bin"],
		database:     database,
		compression:  config["compression"] == "1",
	}
	store.Init()
	return store
}

func (self *articleStore) Compression() bool {
	return self.compression
}

func (self *articleStore) TempDir() string {
	return self.temp
}

// initialize article store
func (self *articleStore) Init() {
	EnsureDir(self.directory)
	EnsureDir(self.temp)
	EnsureDir(self.attachments)
	EnsureDir(self.thumbs)
	if !CheckFile(self.convert_path) {
		log.Fatal("cannot find executable for convert: ", self.convert_path, " not found")
	}
	if !CheckFile(self.ffmpeg_path) {
		log.Fatal("connt find executable for ffmpegthumbnailer: ", self.ffmpeg_path, " not found")
	}
	if !CheckFile(self.sox_path) {
		log.Fatal("connt find executable for sox: ", self.sox_path, " not found")
	}
}

func (self *articleStore) isAudio(fname string) bool {
	for _, ext := range []string{".mp3", ".ogg", ".oga", ".opus", ".flac", ".m4a"} {
		if strings.HasSuffix(strings.ToLower(fname), ext) {
			return true
		}
	}
	return false
}

// is this an image format we need convert for?
func (self *articleStore) isImage(fname string) bool {
	for _, ext := range []string{".gif", ".ico", ".png", ".jpeg", ".jpg", ".png", ".webp"} {
		if strings.HasSuffix(strings.ToLower(fname), ext) {
			return true
		}
	}
	return false

}

func (self *articleStore) GenerateThumbnail(fname string) error {
	outfname := self.ThumbnailFilepath(fname)
	infname := self.AttachmentFilepath(fname)
	var cmd *exec.Cmd
	if self.isImage(fname) {
		cmd = exec.Command(self.convert_path, "-thumbnail", "200", infname, outfname)
	} else if self.isAudio(fname) {
		tmpfname := infname + ".wav"
		cmd = exec.Command(self.ffmpeg_path, "-i", infname, tmpfname)
		out, err := cmd.CombinedOutput()

		if err == nil {
			cmd = exec.Command(self.sox_path, tmpfname, "-n", "spectrogram", "-a", "-d", "0:10", "-r", "-p", "6", "-x", "200", "-y", "150", "-o", outfname)
			out, err = cmd.CombinedOutput()
		}
		if err == nil {
			log.Println("generated audio thumbnail to", outfname)
		} else {
			log.Println("error generating audio thumbnail", err, string(out))
		}
		DelFile(tmpfname)
		return err
	} else {
		cmd = exec.Command(self.ffmpeg_path, "-i", infname, "-vf", "scale=300:200", "-vframes", "1", outfname)
	}
	exec_out, err := cmd.CombinedOutput()
	if err == nil {
		log.Println("made thumbnail for", infname)
	} else {
		log.Println("error generating thumbnail", string(exec_out))
	}
	return err
}

func (self *articleStore) GetAllAttachments() (names []string, err error) {
	var f *os.File
	f, err = os.Open(self.attachments)
	if err == nil {
		names, err = f.Readdirnames(0)
	}
	return
}

func (self *articleStore) OpenMessage(msgid string) (rc io.ReadCloser, err error) {
	fname := self.GetFilename(msgid)
	var f *os.File
	f, err = os.Open(fname)
	if err == nil {
		if self.compression {
			// read gzip header
			var hdr [2]byte
			_, err = f.Read(hdr[:])
			// seek back to beginning
			f.Seek(0, 0)
			if err == nil {
				if hdr[0] == 0x1f && hdr[1] == 0x8b {
					// gzip header detected
					rc, err = gzip.NewReader(f)
				} else {
					// fall back to uncompressed
					rc = f
				}
			} else {
				// error reading file
				f.Close()
				rc = nil
			}
			// will fall back to regular file if gzip header not found
		} else {
			// compression disabled
			// assume uncompressed
			rc = f
		}
	}
	return
}

func (self *articleStore) ReadMessage(r io.Reader) (NNTPMessage, error) {
	return read_message(r)
}

func (self *articleStore) StorePost(nntp NNTPMessage) (err error) {
	var f io.WriteCloser
	f = self.CreateFile(nntp.MessageID())
	if f != nil {
		if self.compression {
			// compress original article with gzip
			var cw *gzip.Writer
			cw, err = gzip.NewWriterLevel(f, gzip.BestSpeed)
			if err == nil {
				err = nntp.WriteTo(cw)
				cw.Close()
				cw.Reset(nil)
				f.Close()
				cw = nil
				f = nil
			}
		} else {
			err = nntp.WriteTo(f)
			f.Close()
			f = nil
		}
	}

	nntp_inner := nntp.Signed()
	if nntp_inner == nil {
		// no inner article
		// store the data in the article
		self.database.RegisterArticle(nntp)
		for _, att := range nntp.Attachments() {
			// save attachment
			self.saveAttachment(att)
		}
	} else {
		// we have inner data
		// store the signed data
		self.database.RegisterArticle(nntp_inner)
		// record a tripcode
		self.database.RegisterSigned(nntp.MessageID(), nntp.Pubkey())
		for _, att := range nntp_inner.Attachments() {
			self.saveAttachment(att)
		}
		nntp_inner.Reset()
	}
	nntp.Reset()
	return
}

func (self *articleStore) saveAttachment(att NNTPAttachment) {
	fpath := att.Filepath()
	upload := self.AttachmentFilepath(fpath)
	if !CheckFile(upload) {
		// attachment does not exist on disk
		f, err := os.Create(upload)
		if f != nil {
			_, err = att.WriteTo(f)
			f.Close()
		}
		if err != nil {
			log.Println("failed to save attachemnt", fpath, err)
		}
	}
	att.Reset()
	self.thumbnailAttachment(fpath)
}

// generate attachment thumbnail
func (self *articleStore) thumbnailAttachment(fpath string) {
	var err error
	thumb := self.ThumbnailFilepath(fpath)
	if !CheckFile(thumb) {
		err = self.GenerateThumbnail(fpath)
		if err != nil {
			log.Println("failed to generate thumbnail for", fpath, err)
		}
	}
}

// get the filepath for an attachment
func (self *articleStore) AttachmentFilepath(fname string) string {
	return filepath.Join(self.attachments, fname)
}

// get the filepath for a thumbanil
func (self *articleStore) ThumbnailFilepath(fname string) string {
	// all thumbnails are jpegs now
	if strings.HasSuffix(fname, ".gif") {
		return filepath.Join(self.thumbs, fname)
	}
	return filepath.Join(self.thumbs, fname+".jpg")
}

// create a file for this article
func (self *articleStore) CreateFile(messageID string) io.WriteCloser {
	fname := self.GetFilename(messageID)
	file, err := os.Create(fname)
	if err != nil {
		log.Println("cannot open file", fname)
		return nil
	}
	return file
}

// create a temp file for inboud articles
func (self *articleStore) CreateTempFile(messageID string) io.WriteCloser {
	fname := self.GetTempFilename(messageID)
	if CheckFile(fname) {
		log.Println(fname, "already open")
		return nil
	}
	file, err := os.Create(fname)
	if err != nil {
		log.Println("cannot open file", fname)
		return nil
	}
	return file
}

// return true if we have an article
func (self *articleStore) HasArticle(messageID string) bool {
	return CheckFile(self.GetFilename(messageID))
}

// get the filename for this article
func (self *articleStore) GetFilename(messageID string) string {
	if !ValidMessageID(messageID) {
		log.Println("!!! bug: tried to open invalid message", messageID, "!!!")
		return ""
	}
	return filepath.Join(self.directory, messageID)
}

// get the filename for this article
func (self *articleStore) GetTempFilename(messageID string) string {
	if !ValidMessageID(messageID) {
		log.Println("!!! bug: tried to open invalid temp message", messageID, "!!!")
		return ""
	}
	return filepath.Join(self.temp, messageID)
}

// loads temp message and deletes old article
func (self *articleStore) ReadTempMessage(messageID string) NNTPMessage {
	fname := self.GetTempFilename(messageID)
	nntp := self.readfile(fname, true)
	DelFile(fname)
	return nntp
}

// read a file give filepath
// parameters are filename and true if it's a temp file
// otherwise parameters are filename and false
func (self *articleStore) readfile(fname string, tmp bool) NNTPMessage {

	file, err := os.Open(fname)
	if err != nil {
		log.Println("store cannot open file", fname, err)
		return nil
	}

	if self.compression && !tmp {
		// we enabled compression and this is not a temp file
		// try compressed version first
		// fall back to uncompressed if failed
		cr, err := gzip.NewReader(file)
		if err == nil {
			// read the message
			message, err := self.ReadMessage(cr)
			// close the compression reader
			cr.Close()
			// close the file
			if err == nil {
				// success
				file.Close()
				return message
			}
		}
		log.Println("store compression enabled but", fname, "doesn't look compressed")
		// decompression failed
		// seek back to the beginning of the file
		file.Seek(0, 0)
	}
	message, err := self.ReadMessage(file)
	file.Close()
	if err == nil {
		return message
	}

	log.Println("store failed to load file", fname, err)
	return nil
}

// load an article
// return nil on failure
func (self *articleStore) GetMessage(messageID string) NNTPMessage {
	return self.readfile(self.GetFilename(messageID), false)
}

func (self *articleStore) GetHeaders(messageID string) (hdr ArticleHeaders) {
	txthdr := self.getMIMEHeader(messageID)
	if txthdr != nil {
		hdr = make(ArticleHeaders)
		for k, val := range txthdr {
			for _, v := range val {
				hdr.Add(k, v)
			}
		}
	}
	return
}

// get article with headers only
func (self *articleStore) getMIMEHeader(messageID string) (hdr textproto.MIMEHeader) {
	if ValidMessageID(messageID) {
		fname := self.GetFilename(messageID)
		f, err := os.Open(fname)
		if f != nil {
			r := bufio.NewReader(f)
			hdr, err = readMIMEHeader(r)
			f.Close()
		}
		if err != nil {
			log.Println("failed to load article headers for", messageID, err)
		}
	}
	return hdr
}

func read_message(r io.Reader) (NNTPMessage, error) {

	msg, err := mail.ReadMessage(r)
	nntp := new(nntpArticle)

	if err == nil {
		body := msg.Body
		hdr := msg.Header
		nntp.headers = ArticleHeaders(hdr)
		content_type := nntp.ContentType()
		media_type, params, err := mime.ParseMediaType(content_type)
		if err != nil {
			log.Println("failed to parse media type", err, "for mime", content_type)
			msg = nil
			nntp.Reset()
			return nil, err
		}
		boundary, ok := params["boundary"]
		if ok {
			partReader := multipart.NewReader(body, boundary)
			msg = nil
			for {
				part, err := partReader.NextPart()
				if err == io.EOF {
					msg = nil
					return nntp, nil
				} else if err == nil {
					hdr := part.Header
					// get content type of part
					part_type := hdr.Get("Content-Type")
					// parse content type
					media_type, _, err = mime.ParseMediaType(part_type)
					if err == nil {
						if media_type == "text/plain" {
							att := readAttachmentFromMimePart(part)
							if att != nil {
								nntp.message = att
							}
						} else {
							// non plaintext gets added to attachments
							att := readAttachmentFromMimePart(part)
							if att != nil {
								nntp.Attach(att)
							}
						}
					} else {
						log.Println("part has no content type", err)
					}
					part.Close()
					part = nil
				} else {
					log.Println("failed to load part! ", err)
					msg = nil
					nntp.Reset()
					return nil, err
				}
			}
		} else if media_type == "message/rfc822" {
			// tripcoded message
			sig := nntp.headers.Get("X-Signature-Ed25519-Sha512", "")
			pk := nntp.Pubkey()
			if pk == "" || sig == "" {
				log.Println("invalid sig or pubkey", sig, pk)
				nntp.Reset()
				nntp = nil
				msg = nil
				return nil, errors.New("invalid headers")
			}
			log.Printf("got signed message from %s", pk)
			pk_bytes := unhex(pk)
			sig_bytes := unhex(sig)
			nntp.signedPart = &nntpAttachment{}
			h := sha512.New()
			mw := io.MultiWriter(h, nntp.signedPart)
			for {
				var n int
				var b [1024]byte
				n, err = body.Read(b[:])
				if err == nil {
					mw.Write(b[:n])
				} else if err == io.EOF {
					err = nil
					break
				} else {
					log.Println("failed to read signed body", err)
					nntp.signedPart.body = nil
					nntp.Reset()
					nntp = nil
					mw = nil
					return nil, err
				}
			}
			mw = nil
			hash := h.Sum(nil)
			h = nil
			log.Printf("hash=%s", hexify(hash))
			log.Printf("sig=%s", hexify(sig_bytes))
			if nacl.CryptoVerifyFucky(hash, sig_bytes, pk_bytes) {
				log.Println("signature is valid :^)")
				msg = nil
				return nntp, nil
			} else {
				log.Println("!!!signature is invalid!!!")
				msg = nil
				nntp.Reset()
				log.Println(nntp.signedPart)
				nntp = nil
				return nil, errors.New("invalid signature")
			}
		} else {
			// plaintext attachment
			var buff [1024]byte
			str := ""
			for {
				var n int
				n, err = body.Read(buff[:])
				if err != nil {
					if err == io.EOF {
						err = nil
					}
					break
				}
				str += string(buff[:n])
			}
			nntp.message = createPlaintextAttachment(str)
			msg = nil
			return nntp, err
		}
	} else {
		log.Println("failed to read message", err)
		msg = nil
		nntp.Reset()
		return nil, err
	}
	msg = nil
	return nntp, err
}
