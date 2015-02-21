//
// message.go
//
package main

import (
  "bufio"
  "bytes"
  "database/sql"
  "io"
  "log"
  "mime"
  "mime/multipart"
  "os"
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
  PubKey string
  Signature string
  Posted time.Time
  Message string
  Path string
  ContentType string
  Sage bool
  Attachments []NNTPAttachment
}

// load from file
func (self *NNTPMessage) Load(file *os.File, loadBody bool) bool {
  self.Please = "post"
  reader := bufio.NewReader(file)
  var idx int
  for {
    line, err := reader.ReadString('\n')
    if err != nil {
      log.Println("failed to read message", file.Name())
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
    } else if strings.HasPrefix(lowline, "reference: ") {
      self.Reference = line[11:llen-1]
    } else if strings.HasPrefix(lowline, "from: ") {
      line = line[6:llen-1]
      llen = len(line)
      idx = strings.LastIndex(line, " ")
      if idx < llen && idx > 0 {
        self.Name = line[:idx]
        self.Email = line[idx+1:llen-1]
      } else {
        self.Name = line
      }
    } else if strings.HasPrefix(lowline, "x-pubkey-ed25519: ") {
      self.PubKey = line[18:llen-1] 
    } else if strings.HasPrefix(lowline, "x-signature-ed25519-sha512: ") {
      self.Signature = line[28:llen-1]
    } else if strings.HasPrefix(lowline, "date: ") {
      date, err := time.Parse(time.RFC1123Z, line[6:llen-1])
      if err == nil {
        self.Posted = date
      }
    } else if strings.HasPrefix(lowline, "content-type: ") {
      self.ContentType = line[14:llen-1]
    }
  }
  if !loadBody || self.Newsgroup == "ano.paste" {
    return true
  }
  var bodybuff bytes.Buffer
  _, err := bodybuff.ReadFrom(reader)
  if err != nil {
    log.Println(self.MessageID, "failed to load body", err) 
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
      parts[idx].Mime = part.Header.Get("Content-Type")
      _, err = buff.ReadFrom(part)
      if err != nil {
        log.Println("failed to load attachment for", self.MessageID, err)
        return false
      }
      parts[idx].Data = buff.String()
      idx += 1
    }
    if idx > 0 {
      self.Attachments = make([]NNTPAttachment, idx)
      for idx = range(self.Attachments) {
        self.Attachments[idx] = parts[idx]
      }
    }
  } else {
    for {
      line, err := reader.ReadString('\n')
      if err == io.EOF {
        return true
      } 
      if err != nil {
        log.Println("failed to load message", self.MessageID, err)
      }
      self.Message += line
    }
  }
  return true
}

// add to database
func (self *NNTPMessage) Save(database *sql.DB) {
  var userid string
  err := database.QueryRow(`INSERT INTO nntp_posts(
    messageID, newsgroup, subject, pubkey, message, parent)
    VALUES( $1, $2, $3, $4, $5, $6) 
    RETURNING id`, self.MessageID, self.Newsgroup, self.Subject, self.PubKey, self.Message, self.Reference).Scan(&userid)
  if err != nil {
    log.Println("failed to save post to database", err)
    return
  }
  log.Println("inserted post with UUID", userid)
}