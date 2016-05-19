package store

import (
	"encoding/base32"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/crypto"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// filesystem storage of nntp articles and attachments
type FilesystemStorage string

func (fs FilesystemStorage) String() string {
	return string(fs)
}

// ensure the filesystem storage exists and is well formed and read/writable
func (fs FilesystemStorage) Ensure() (err error) {
	_, err = os.Stat(fs.String())
	if err == os.ErrNotExist {
		// directory does not exist, create it
		err = os.Mkdir(fs.String(), 0755)
		if err != nil {
			log.WithFields(log.Fields{
				"pkg":      "fs-store",
				"filepath": fs.String(),
			}).Error("failed to ensure directory", err)
			// failed to create initial directory
			return
		}
	}

	// ensure subdirectories
	for _, subdir := range []string{"att", "thm", "articles", "tmp"} {
		fpath := filepath.Join(fs.String(), subdir)
		_, err = os.Stat(fpath)
		if err == os.ErrNotExist {
			// make subdirectory
			err = os.Mkdir(fpath, 0755)
			if err != nil {
				log.WithFields(log.Fields{
					"pkg":      "fs-store",
					"filepath": fpath,
				}).Error("failed to ensure sub-directory", err)
				// failed to create subdirectory
				return
			}
		}
	}
	return
}

// get the temp file directory
func (fs FilesystemStorage) TempDir() string {
	return filepath.Join(fs.String(), "tmp")
}

// get the directory path for attachments
func (fs FilesystemStorage) AttachmentDir() string {
	return filepath.Join(fs.String(), "att")
}

// get the directory path for articles
func (fs FilesystemStorage) ArticleDir() string {
	return filepath.Join(fs.String(), "articles")
}

// get a temporary file we can use for read/write that deletes itself on close
func (fs FilesystemStorage) obtainTempFile() (f *os.File, err error) {
	fname := fmt.Sprintf("tempfile-%x-%d", crypto.RandBytes(4), time.Now().Unix())
	log.WithFields(log.Fields{
		"pkg":      "fs-store",
		"filepath": fname,
	}).Debug("opening temp file")
	f, err = os.OpenFile(filepath.Join(fs.TempDir(), fname), os.O_RDWR, os.ModeTemporary)
	return
}

// store an article from a reader to disk
func (fs FilesystemStorage) StoreArticle(r io.Reader, msgid string) (fpath string, err error) {
	err = fs.HasArticle(msgid)
	if err == nil {
		// discard the body as we have it stored already
		_, err = io.Copy(ioutil.Discard, r)
		log.WithFields(log.Fields{
			"pkg":   "fs-store",
			"msgid": msgid,
		}).Debug("discard article")
	} else if err == ErrNoSuchArticle {
		log.WithFields(log.Fields{
			"pkg":   "fs-store",
			"msgid": msgid,
		}).Debug("storing article")
		// don't have an article with this message id, write it to disk
		var f *os.File
		f, err = os.OpenFile(fpath, os.O_WRONLY, 0700)
		if err == nil {
			// file opened okay, defer the close
			defer f.Close()
			// write to disk
			log.WithFields(log.Fields{
				"pkg":   "fs-store",
				"msgid": msgid,
			}).Debug("writing to disk")
			var n int64
			n, err = io.Copy(f, r)
			if err == nil {
				log.WithFields(log.Fields{
					"pkg":     "fs-store",
					"msgid":   msgid,
					"written": n,
				}).Debug("wrote article to disk")
			} else {
				log.WithFields(log.Fields{
					"pkg":     "fs-store",
					"msgid":   msgid,
					"written": n,
				}).Error("write to disk failed")
			}
		} else {
			log.WithFields(log.Fields{
				"pkg":   "fs-store",
				"msgid": msgid,
			}).Error("did not open file for storage", err)
		}
	}
	return
}

// check if we have the artilce with this message id
func (fs FilesystemStorage) HasArticle(msgid string) (err error) {
	fpath := fs.ArticleDir()
	fpath = filepath.Join(fpath, msgid)
	log.WithFields(log.Fields{
		"pkg":      "fs-store",
		"msgid":    msgid,
		"filepath": fpath,
	}).Debug("check for article")
	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		err = ErrNoSuchArticle
	}
	return
}

// store attachment onto filesystem
func (fs FilesystemStorage) StoreAttachment(r io.Reader, filename string) (fpath string, err error) {
	// open temp file for storage
	var tf *os.File
	tf, err = fs.obtainTempFile()
	if err == nil {
		// we have the temp file

		// close tempfile when done
		defer tf.Close()

		// create hasher
		h := crypto.Hash()
		// create multiwriter
		mw := io.MultiWriter(tf, h)

		log.WithFields(log.Fields{
			"pkg":      "fs-store",
			"filename": filename,
		}).Debug("writing to disk")
		var n int64
		// write all of the reader to the multiwriter
		n, err = io.Copy(mw, r)

		if err == nil {
			// successful write

			// get file checksum
			d := h.Sum(nil)

			// rename file to hash + extension from filename
			fpath = base32.StdEncoding.EncodeToString(d) + filepath.Ext(filename)
			fpath = filepath.Join(fs.AttachmentDir(), fpath)

			_, err = os.Stat(fpath)
			// is that file there?
			if os.IsNotExist(err) {
				// it's not there, let's write it
				var f *os.File
				f, err = os.OpenFile(fpath, os.O_WRONLY, 0755)
				if err == nil {
					// file opened
					defer f.Close()
					// seek to beginning of tempfile
					tf.Seek(0, os.SEEK_SET)
					// write all of the temp file to the storage file
					n, err = io.Copy(f, tf)
					// if err == nil by here it's all good
					l := log.WithFields(log.Fields{
						"pkg":      "fs-store",
						"filename": filename,
						"hash":     d,
						"filepath": fpath,
						"size":     n,
					})
					if err == nil {
						l.Debug("wrote attachment to disk")
					} else {
						l.Error("failed to write attachment to disk", err)
					}
				} else {
					log.WithFields(log.Fields{
						"pkg":      "fs-store",
						"filename": filename,
						"hash":     d,
						"filepath": fpath,
					}).Error("failed to open file")
				}
			} else {
				log.WithFields(log.Fields{
					"pkg":      "fs-store",
					"filename": filename,
					"hash":     d,
					"filepath": fpath,
					"size":     n,
				}).Debug("attachment exists on disk")
			}
		}
	} else {
		log.WithFields(log.Fields{
			"pkg":      "fs-store",
			"filename": filename,
		}).Error("cannot open temp file for attachment", err)
	}
	return
}

// create a new filesystem storage directory
// ensure directory and subdirectories
func NewFilesytemStorage(dirname string) (fs FilesystemStorage, err error) {
	dirname, err = filepath.Abs(dirname)
	if err == nil {
		log.WithFields(log.Fields{
			"pkg":      "fs-store",
			"filepath": dirname,
		}).Info("Creating New Filesystem Storage")
		fs = FilesystemStorage(dirname)
		err = fs.Ensure()
	}
	return
}
