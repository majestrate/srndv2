package store

import (
	"encoding/base32"
	"fmt"
	"github.com/dchest/blake256"              // for filehash
	"github.com/majestrate/srndv2/lib/crypto" // for random

	"io"
	"os"
	"path/filepath"
	"time"
)

//
// nntp article filesystem storage
//

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

func (fs FilesystemStorage) AttachmentDir() string {
	return filepath.Join(fs.String(), "att")
}

// get a temporary file we can use for read/write that deletes itself on close
func (fs FilesystemStorage) obtainTempFile() (f *os.File, err error) {
	fname := fmt.Sprintf("tempfile-%x-%d", crypto.RandBytes(4), time.Now().Unix())
	f, err = os.OpenFile(filepath.Join(fs.TempDir(), fname), os.O_RDWR, os.ModeTemporary)
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
		h := blake256.New()
		// create multiwriter
		mw := io.MultiWriter(tf, h)

		// write all of the reader to the multiwriter
		_, err = io.Copy(mw, r)

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
					_, err = io.Copy(f, tf)
					// if err == nil by here it's all good
				}
			}
		}
	}
	return
}

// create a new filesystem storage directory
// ensure directory and subdirectories
func NewFilesytemStorage(dirname string) (fs FilesystemStorage, err error) {
	dirname, err = filepath.Abs(dirname)
	if err == nil {
		fs = FilesystemStorage(dirname)
		err = fs.Ensure()
	}
	return
}
