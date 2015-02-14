//
// nntp.go
//
package main

import (
	"bufio"
	"log"
	"net"
	"strings"
)

type ConnectionInfo struct {
	mode string
	newsgroup string
	allowsPosting bool 
	supportsStream bool 
}

type NNTPConnection struct {
	conn net.Conn
	reader *bufio.Reader
	inbound bool
	debug bool
	info *ConnectionInfo
}

func (self *NNTPConnection) HandleOutbound(d *NNTPDaemon) {
	line := self.ReadLine()
	self.info.allowsPosting = strings.HasPrefix("200 ", line)
	// they allow posting
	// send capabilities command
	self.SendLine("CAPABILITIES")
	
	// get capabilites
	for {
		line = strings.ToLower(self.ReadLine())
		if line == "." {
			// done reading capabilities
			break
		}
		if line == "streaming" {
			self.info.supportsStream = true
		} else if line == "postihavestreaming" {
			self.info.supportsStream = true
		}
	}

	// if they support streaming and allow posting continue
	// otherwise quit
	if ! ( self.info.supportsStream && self.info.allowsPosting ) {
		self.Quit()
		return
	}
	self.SendLine("MODE STREAM")
	line = self.ReadLine()
	if strings.HasPrefix("203 ", line) {
		self.info.mode = "stream"
		log.Println("streaming mode activated")
	} else {
		self.Quit()
		return
	}
	// mainloop
	for  {
		line = self.ReadLine()
	}

}

// handle inbound connection
func (self *NNTPConnection) HandleInbound(d *NNTPDaemon) {
	log.Println("Incoming nntp connection from", self.conn.RemoteAddr())
	// send welcome
	self.SendLine("200 ayy lmao we are SRNd2, posting allowed")
	for {
		line := self.ReadLine()
    if len(line) == 0 {
			break
		}
		commands := strings.Split(line, " ")
		cmd := strings.ToLower(commands[0])
		if cmd == "CAPABILITIES" {
			self.sendCapabilities()
		}
	}
	self.Close()
}

func (self *NNTPConnection) sendCapabilities() {
	self.SendLine("101 we can do stuff")
	self.SendLine("STREAMING")
	self.SendLine("POST")
	self.SendLine(".")
}

func (self *NNTPConnection) Quit() {
	if ! self.inbound {
		self.SendLine("QUIT")
		_ = self.ReadLine()
	}
	self.Close()
}

func (self *NNTPConnection) ReadLine() string {
	line, err := self.reader.ReadString('\n')
	if err != nil {
		return ""
	}
	line = strings.Replace(line, "\n", "", -1)
	line = strings.Replace(line, "\r", "", -1)
	if self.debug {
		log.Println(self.conn.RemoteAddr(), "recv line", line)
	}
	return line
}

// send a line
func (self *NNTPConnection) SendLine(line string) {
	if self.debug {
		log.Println(self.conn.RemoteAddr(), "send line", line)
	}
	self.conn.Write([]byte(line+"\r\n"))
}

// close the connection
func (self *NNTPConnection) Close() {
	err := self.conn.Close()
	if err != nil {
		log.Println(self.conn.RemoteAddr(), err)
	}
	log.Println(self.conn.RemoteAddr(), "Closed Connection")
}
