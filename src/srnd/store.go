//
// store.go
//

package srnd

import (
  "bufio"
  "bytes"
  //"encoding/base64"
  "errors"
  "fmt"
  "io"
  //"io/ioutil"
  "log"
  "os"
  "path/filepath"
  "strings"
)

type ArticleStore interface {
  MessageReader
  MessageWriter
  MessageVerifier
  
  // get the filepath for an attachment
  AttachmentFilepath(fname string) string
  // get the filepath for an attachment's thumbnail
  ThumbnailFilepath(fname string) string
  // do we have this article?
  HasArticle(msgid string) bool
  // create a file for a message
  CreateFile(msgid string) io.WriteCloser
  // create a file for a temp message
  CreateTempFile(msgid string) io.WriteCloser
  // get the filename of a message
  GetFilename(msgid string) string
  // get the filename of a temp message
  GetTempFilename(msgid string) string
  // Get a message given its messageid
  GetMessage(msgid string) NNTPMessage
  // get a temp message given its messageid
  // temp message is deleted once read
  ReadTempMessage(msgid string) NNTPMessage
  // store a post
  StorePost(nntp NNTPMessage) error
  // get article headers only
  GetHeaders(msgid string) ArticleHeaders
}
type articleStore struct {
  directory string
  temp string
  attachments string
  thumbs string
  database Database
}


func createArticleStore(config map[string]string, database Database) ArticleStore {
  store := articleStore{
    directory: config["store_dir"],
    temp: config["incoming_dir"],
    attachments: config["attachments_dir"],
    thumbs: config["thumbs_dir"],
    database: database,
  }
  store.Init()
  return store
}

// initialize article store
func (self articleStore) Init() {
  EnsureDir(self.directory)
  EnsureDir(self.temp)
  EnsureDir(self.attachments)
  EnsureDir(self.thumbs)
}

func (self articleStore) StorePost(nntp NNTPMessage) (err error) {
  f := self.CreateFile(nntp.MessageID())
  if f != nil {
    err = self.WriteMessage(nntp, f)
    f.Close()
  }
  return 
}

func (self articleStore) ReadMessage(r io.Reader) (NNTPMessage, error) {
  nntp := nntpArticle{headers: make(ArticleHeaders)}
  reader := bufio.NewReader(r)
  var err error
  // read headers
  for {
    l, err := reader.ReadString('\n')
    if err == io.EOF {
      return nil, errors.New("got EOF while reading headers")
    } else if err != nil {
      return nil, err
    }
    linelen := len(l)
    var line string
    if strings.HasPrefix(l, "\r\n") {
      line = l[:linelen-2]
    } else {
      line = l[:linelen-1]
    }
    // end of headers?
    if len(line) == 0 {
      break
    }
    colonIdx := strings.Index(line, ": ")
    if colonIdx > 1 {
      headername := line[:colonIdx]
      headerval := line[colonIdx+2:]
      nntp.headers[headername] = headerval
    } else {
      // invalid line
      return nil, errors.New("invalid header line: "+l)
    }
  }
  var body bytes.Buffer
  _, err = io.Copy(&body, reader)
  nntp.message = body.String()
  return nntp, err
}


func (self articleStore) WriteMessage(nntp NNTPMessage, wr io.Writer) (err error) {
  // write headers
  for hdr, hdr_val := range(nntp.Headers()) {
    _, err = io.WriteString(wr, fmt.Sprintf("%s: %s\n", hdr, hdr_val))
    if err != nil {
      return
    }
  }
  // done headers
  _, err = io.WriteString(wr, "\n")
  if err != nil {
    return
  }
  // write body
  err = nntp.WriteBody(wr)
  return
}



func (self articleStore) VerifyMessage(nntp NNTPMessage) (NNTPMessage, error) {
  // TODO: implement
  return nntp, nil
}

// get the filepath for an attachment
func (self articleStore) AttachmentFilepath(fname string) string {
  return filepath.Join(self.attachments, fname)
}

// get the filepath for a thumbanil
func (self articleStore) ThumbnailFilepath(fname string) string {
  return filepath.Join(self.thumbs, fname)
}

// create a file for this article
func (self articleStore) CreateFile(messageID string) io.WriteCloser {
  fname := self.GetFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Println("cannot open file", fname)
    return nil
  }
  return file
}

// create a temp file for inboud articles
func (self articleStore) CreateTempFile(messageID string) io.WriteCloser {
  fname := self.GetTempFilename(messageID)
  file, err := os.Create(fname)
  if err != nil {
    log.Println("cannot open file", fname)
    return nil
  }
  return file
}

// return true if we have an article
func (self articleStore) HasArticle(messageID string) bool {
  return self.database.HasArticle(messageID)
}

// get the filename for this article
func (self articleStore) GetFilename(messageID string) string {
  return filepath.Join(self.directory, messageID)
}

// get the filename for this article
func (self articleStore) GetTempFilename(messageID string) string {
  return filepath.Join(self.temp, messageID)
}

// loads temp message and deletes old article
func (self articleStore) ReadTempMessage(messageID string) NNTPMessage {
  fname := self.GetTempFilename(messageID)
  nntp := self.readfile(fname)
  DelFile(fname)
  return nntp
}

// read a file give filepath
func (self articleStore) readfile(fname string) NNTPMessage {
  
  file, err := os.Open(fname)
  if err != nil {
    log.Println("store cannot open file",fname)
    return nil
  }
  message, err := self.ReadMessage(file)
  file.Close()
  if err == nil {
    return message
  }
  
  log.Println("failed to load file", fname)
  return nil
}

// get the replies for a thread
func (self articleStore) GetThreadReplies(messageID string, last int) []NNTPMessage {
  var repls []NNTPMessage
  if self.database.ThreadHasReplies(messageID) {
    rpls := self.database.GetThreadReplies(messageID, last)
    if rpls == nil {
      return repls
    }
    for _, rpl := range rpls {
      msg := self.GetMessage(rpl)
      if msg == nil {
        log.Println("cannot get message", rpl)
      } else { 
        repls = append(repls, msg)
      }
    }
  }
  return repls
}

// load an article
// return nil on failure
func (self articleStore) GetMessage(messageID string) NNTPMessage {
  return self.readfile(self.GetFilename(messageID))
}

// get article with headers only
func (self articleStore) GetHeaders(messageID string) ArticleHeaders {
  // TODO: don't load the entire body
  nntp := self.readfile(self.GetFilename(messageID))
  return nntp.Headers()
}


