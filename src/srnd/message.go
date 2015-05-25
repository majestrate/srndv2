//
// message.go
//
package srnd

import (
  "bufio"
  "bytes"
  "crypto/rand"
  "crypto/sha512"
  "database/sql"
  "encoding/hex"
  "fmt"
  "github.com/majestrate/srndv2/src/nacl"
  "io"
  "log"
  "mime"
  "mime/multipart"
  "net/textproto"
  "path/filepath"
  "strings"
  "time"
)

type NNTPAttachment struct {
  Mime string
  Name string
  Extension string
  Data string
}

type NNTPMessage struct {
  Please string
  MessageID string
  Reference string
  Newsgroup string
  Name string
  Email string
  Subject string
  Key string
  Signature string
  Posted int64
  Message string
  Path string
  ContentType string
  Sage bool
  OP bool
  Attachments []NNTPAttachment
  Moderation string
}

// verify any signatures
// if no signatures are found this does nothing and returns true
// if signatures are found it returns true if they are valid, otherwise false
func (self *NNTPMessage) VerifySignature() bool {
  if len(self.Signature) > 0 && len(self.Key) > 0 && len(self.Moderation) > 0 {
    // SRNd is wierd 
    // replace <LF> with <CR><LF> so that sigs work
    mod := strings.Replace(self.Moderation, "\n", "\r\n", -1)
    buff := []byte(mod)
    // trim off the last stuff
    buff = buff[:len(buff)-2]
    // sum the mod message body
    hash := sha512.Sum512(buff)
    msg_hash := hash[:]
    // extract sig and pubkey
    sig_bytes, err := hex.DecodeString(self.Signature)
    if err != nil {
      log.Println("invalid signature format", err)
      return false
    }
    pk_bytes, err := hex.DecodeString(self.Key)
    if err != nil {
      log.Println("invalid pubkey format", err)
      return false
    }
    log.Printf("verify pubkey message from %s", self.Key)
    // uses fucky crypto_sign_open instead of detached sigs wtf
    var smsg bytes.Buffer
    smsg.Write(sig_bytes)
    smsg.Write(msg_hash)
    if nacl.CryptoVerify(smsg.Bytes(), pk_bytes) {
      log.Printf("%s verified", self.MessageID)
      return true
    }
    log.Println("%s has invalid signature", self.MessageID)
    return false
  }
  return true
}

func (self *NNTPMessage) WriteTo(w io.WriteCloser, delim string) (err error) {
  var r [30]byte
  io.ReadFull(rand.Reader, r[:])
  boundary := fmt.Sprintf("%x", r[:])

  writer := NewLineWriter(w, delim)
  
  // content type header
  // overwrite if we have attachments
  if len(self.Attachments) > 0 {
    // mime header
    io.WriteString(writer, "Mime-Version: 1.0")
    self.ContentType = fmt.Sprintf("multipart/mixed; boundary=\"%s\"", boundary)
  }
  io.WriteString(writer, fmt.Sprintf("Content-Type: %s", self.ContentType))
  // from header
  // TODO: sanitize this
  name := self.Name
  email := self.Email
  io.WriteString(writer, fmt.Sprintf("From: %s <%s>", name, email))
  // date header
  date := time.Unix(self.Posted, 0).UTC()
  io.WriteString(writer, fmt.Sprintf("Date: %s", date.Format(time.RFC1123Z)))
  // write key / sig headers
  if len(self.Key) > 0 && len(self.Signature) > 0 {
    io.WriteString(writer, fmt.Sprintf("X-pubkey-ed25519: %s", self.Key))
    io.WriteString(writer, fmt.Sprintf("X-signature-ed25519-sha512: %s", self.Signature))
  }
  
  // newsgroups header
  io.WriteString(writer, fmt.Sprintf("Newsgroups: %s", self.Newsgroup))
  // subject header
  io.WriteString(writer, fmt.Sprintf("Subject: %s", self.Subject))
  // message id header
  io.WriteString(writer, fmt.Sprintf("Message-ID: %s", self.MessageID))

  // references header
  if len(self.Reference) > 0 {
    io.WriteString(writer, fmt.Sprintf("References: %s", self.Reference))
  }
  // path header
  io.WriteString(writer, fmt.Sprintf("Path: %s", self.Path))

  // TODO: sign/verify

  // header done
  _, err = io.WriteString(writer, "")
  if err != nil {
    return err
  }

  // this is a mod message
  if len(self.Moderation) > 0 {
    _, err = io.WriteString(writer, self.Moderation)
    return err
  }
  
  // do we have attachments?
  if len(self.Attachments) > 0 {
    // ya we have files
    io.WriteString(writer, "SRNDv2 Multipart UGUU")
    mwriter := multipart.NewWriter(writer)
    mwriter.SetBoundary(boundary)
    // message
    hdr := make(textproto.MIMEHeader)
    hdr.Set("Content-Type", "text/plain; charset=UTF-8")
    hdr.Set("Content-Transfer-Encoding", "8bit")
    part, _ := mwriter.CreatePart(hdr)
    io.WriteString(part, self.Message)
    // files
    for idx := range(self.Attachments) {
      att := self.Attachments[idx]
      hdr := make(textproto.MIMEHeader)
      hdr.Set("Content-Type", att.Mime)
      hdr.Set("Content-Disposition", "attachment")
      hdr.Add("Content-Disposition", fmt.Sprintf("filename=\"%s\"", att.Name))
      hdr.Set("Content-Transfer-Encoding", "base64")
      part, _ := mwriter.CreatePart(hdr)
      // write it to our mime message
      io.WriteString(part, att.Data)
      
    }
    mwriter.Close()
  } else {
    // nope we have no files
    // write out a plain response
    _, err = io.WriteString(writer, self.Message)
  }
  return err
}

// load from file
func (self *NNTPMessage) Load(file io.Reader, loadBody bool) bool {
  self.Please = "post"
  reader := bufio.NewReader(file)
  var idx int
  for {
    line, err := reader.ReadString('\n')
    if err != nil {
      log.Println("failed to read message", err)
      return false
    }
    // we are done reading headers
    // break out
    if line == "\n" {
      break
    }
    lowline := strings.ToLower(line)
    llen := len(line)
    // check newsgroup header
    if strings.HasPrefix(lowline, "newsgroups: ") {
      newsgroups:= line[12:llen-1]
      idx = strings.Index(newsgroups, ",")
      if idx != -1 {
        newsgroups = newsgroups[:idx]
      }
      self.Newsgroup = newsgroups
    } else if strings.HasPrefix(lowline, "x-sage: ") {
      self.Sage = true
    } else if strings.HasPrefix(lowline, "message-id: ") {
      self.MessageID = line[12:llen-1]
    } else if strings.HasPrefix(lowline, "subject: ") {
      self.Subject = line[9:llen-1]
    } else if strings.HasPrefix(lowline, "path: ") {
      self.Path = line[6:llen-1]
    } else if strings.HasPrefix(lowline, "references: ") {
      self.Reference = line[12:llen-1]
    } else if strings.HasPrefix(lowline, "from: ") {
      line = line[6:llen-1]
      llen = len(line)
      idx = strings.LastIndex(line, " ")
      if idx + 2 < llen && idx > 0 {
        self.Name = line[:idx]
        self.Email = line[idx+2:llen-1]
      } else {
        self.Name = line
      }
    } else if strings.HasPrefix(lowline, "x-pubkey-ed25519: ") {
      self.Key = line[18:llen-1] 
    } else if strings.HasPrefix(lowline, "x-signature-ed25519-sha512: ") {
      self.Signature = line[28:llen-1]
    } else if strings.HasPrefix(lowline, "date: ") {
      date, err := time.Parse(time.RFC1123Z, line[6:llen-1])
      if err == nil {
        self.Posted = date.Unix()
      }
    } else if strings.HasPrefix(lowline, "content-type: ") {
      self.ContentType = line[14:llen-1]
    }
  }
  // TODO: allow pastebin
  if !loadBody || self.Newsgroup == "ano.paste" {
    return true
  }

  var bodybuff bytes.Buffer
  _, err := bodybuff.ReadFrom(reader)

  if err != nil {
    log.Println(self.MessageID, "failed to load body", err) 
  }
  
  // this is a mod message
  // treat it differently because signatures
  if self.Newsgroup == "ctl" {
    // read the rest of the message
    self.Moderation = bodybuff.String()
    return true
  }
  
  if self.ContentType == "" {
    self.Message = bodybuff.String()
    return true
  }
  
  mediaType, params, err := mime.ParseMediaType(self.ContentType)
  if err != nil {
    log.Println(self.MessageID, "error loading body", err)
    return false
  }
  semi_idx := strings.Index(self.ContentType, ";")
  if semi_idx > 0 {
    self.ContentType = self.ContentType[:semi_idx]
  }
  bodyreader := bytes.NewReader(bodybuff.Bytes())
  parts := make([]NNTPAttachment, 32)
  idx = 0
  if strings.HasPrefix(mediaType, "multipart/") {
    mr := multipart.NewReader(bodyreader, params["boundary"])
    for {
      var buff bytes.Buffer
      if idx >= 32 {
        log.Println("too many parts in", self.MessageID)
        return false
      }
      part, err := mr.NextPart()
      if err == io.EOF {
        break
      }
      if err != nil {
        log.Println("failed to read multipart message in", self.MessageID, err)
        return true
      }
      fname := part.FileName()
      parts[idx].Name = fname
      parts[idx].Extension = filepath.Ext(fname)
      mime := part.Header.Get("Content-Type")
      semi_idx = strings.Index(mime, ";")
      if semi_idx > 0 {
        mime = mime[:semi_idx]
      }
      parts[idx].Mime = mime
      _, err = buff.ReadFrom(part)
      if err != nil {
        log.Println("failed to load attachment for", self.MessageID, err)
        return false
      }
      parts[idx].Data = buff.String()

      if parts[idx].Mime == "text/plain" {
        self.Message += parts[idx].Data
        parts[idx].Data = ""
      } 
      
      idx += 1

    }
    
    self.Attachments = make([]NNTPAttachment, idx)

    counter := 0
    for i := range(parts) {
      part := parts[i]
      if len(part.Data) > 0 {
        self.Attachments[counter] = parts[i]
        counter ++        
      }
    }
    self.Attachments = self.Attachments[:idx-counter]
  } else {
  
    self.Message = bodybuff.String()
    
  }
  return true
}

// add to database
func (self *NNTPMessage) Save(database *sql.DB) {
  var userid string
  err := database.QueryRow(`INSERT INTO nntp_posts(
    messageID, newsgroup, subject, pubkey, message, parent)
    VALUES( $1, $2, $3, $4, $5, $6) 
    RETURNING id`, self.MessageID, self.Newsgroup, self.Subject, self.Key, self.Message, self.Reference).Scan(&userid)
  if err != nil {
    log.Println("failed to save post to database", err)
    return
  }
  log.Println("inserted post with UUID", userid)
}
