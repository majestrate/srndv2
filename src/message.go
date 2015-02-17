//
// message.go
//
package main

import (
	"bufio"
	"log"
	"os"
	"strings"
	"time"
)

type NNTPAttachment struct {
	filename string
	data string
}

type NNTPMessage struct {
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

// load headers from file
func (self *NNTPMessage) LoadHeaders(file *os.File) bool {
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
			if 1 + idx < llen {
				self.Name = line[:idx]
				self.Email = line[idx+2:llen-1]
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
	
	return true
		
}

// load body
// TODO: implement
func (self *NNTPMessage) LoadBody(file *os.File) bool {
	return false
}
