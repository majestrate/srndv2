//
// nntp frontend
//

package srnd

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
)

type nntpFrontend struct {
	postsChan   chan NNTPMessage
	discardChan chan NNTPMessage
	store       ArticleStore
	db          Database
	bindaddr    string
}

func NewNNTPFrontend(d *NNTPDaemon, bindaddr string) Frontend {
	return nntpFrontend{
		postsChan:   make(chan NNTPMessage),
		discardChan: make(chan NNTPMessage),
		store:       d.store,
		db:          d.database,
		bindaddr:    bindaddr,
	}
}

func (self nntpFrontend) AllowNewsgroup(group string) bool {
	return strings.HasPrefix(group, "overchan.") && newsgroupValidFormat(group)
}

func (self nntpFrontend) NewPostsChan() chan NNTPMessage {
	return self.postsChan
}

func (self nntpFrontend) PostsChan() chan NNTPMessage {
	return self.discardChan
}

func (self nntpFrontend) Regen(e ArticleEntry) {
	// this does nuthin
}

func (self nntpFrontend) Mainloop() {

	// forever discard incoming messages
	go func() {
		for {
			_ = <-self.discardChan
		}
	}()

	sock, err := net.Listen("tcp", self.bindaddr)
	if err != nil {
		// could not bind
		log.Fatalf("could not bind nntp frontend: %s", err.Error())
	}
	// accept incoming connections
	for {
		conn, err := sock.Accept()
		if err == nil {
			go self.handle_connection(conn)
		}
	}
}

func (self nntpFrontend) handle_connection(sock net.Conn) {
	log.Println("incoming nntp frontend connection", sock.RemoteAddr())
	// wrap the socket
	r := textproto.NewReader(bufio.NewReader(sock))
	w := textproto.NewWriter(bufio.NewWriter(sock))
	var line, newsgroup string
	// write out greeting
	err := w.PrintfLine("201 ayyy srndv2 nntp frontend here, posting disallowed")
	for {
		if err != nil {
			// abort it
			log.Println("error handling nntp frontend connection", err)
			break
		}
		line, err = r.ReadLine()
		lline := strings.ToLower(line)

		// we are in reader mode
		if lline == "quit" {
			break
		} else if strings.HasPrefix(lline, "newsgroups ") {
			// handle newsgroups command
			// TODO: don't ignore dates
			w.PrintfLine("231 list of newsgroups follows")
			groups := self.db.GetAllNewsgroups()
			dw := w.DotWriter()
			for _, group := range groups {
				last, first, err := self.db.GetLastAndFirstForGroup(group)
				if err == nil {
					io.WriteString(dw, fmt.Sprintf("%s %d %d y\r\n", group, last, first))
				} else {
					log.Println("cannot get last/first ids for group", group, err)
				}
			}
			dw.Close()
		} else if lline == "list" {
			w.PrintfLine("215 list of newsgroups follows")
			// handle list command
			groups := self.db.GetAllNewsgroups()
			dw := w.DotWriter()
			for _, group := range groups {
				last, first, err := self.db.GetLastAndFirstForGroup(group)
				if err == nil {
					io.WriteString(dw, fmt.Sprintf("%s %d %d y\r\n", group, last, first))
				} else {
					log.Println("cannot get last/first ids for group", group, err)
				}
			}
			dw.Close()
		} else if strings.HasPrefix(lline, "group ") {
			// handle group command
			newsgroup = lline[6:]
			if self.db.HasNewsgroup(newsgroup) {
				article_count := self.db.CountPostsInGroup(newsgroup, 0)
				last, first, err := self.db.GetLastAndFirstForGroup(newsgroup)
				if err == nil {
					w.PrintfLine("211 %d %d %d %s", article_count, first, last, newsgroup)
				} else {
					w.PrintfLine("500 internal error, %s", err.Error())
				}
			} else {
				w.PrintfLine("411 no such news group")
				newsgroup = ""
			}
		} else if lline == "list overview.fmt" {
			// handle overview listing
			if newsgroup == "" {
				// no newsgroup
				w.PrintfLine("412 No newsgorup selected")
			} else {
				// assume we got the newsgroup set
				dw := w.DotWriter()
				// write out format
				// TODO: hard coded
				io.WriteString(dw, "215 Order of fields in overview database.\r\n")
				io.WriteString(dw, "Subject:\r\nFrom:\r\nDate:\r\nMessage-ID:\r\nRefernces:\r\n")
				dw.Close()
			}
		} else if strings.HasPrefix(lline, "xover ") {
			if newsgroup == "" {
				w.PrintfLine("412 No newsgroup selected")
			} else {
				// handle xover command
				// every article
				models, err := self.db.GetPostsInGroup(newsgroup)
				if err == nil {
					w.PrintfLine("224 Overview information follows")

					dw := w.DotWriter()
					for idx, model := range models {
						io.WriteString(dw, fmt.Sprintf("%.6d\t%s\t\"%s\" <%s@%s>\t%s\t%s\t%s\r\n", idx+1, model.Subject(), model.Name(), model.Name(), model.Frontend(), model.Date(), model.MessageID(), model.Reference()))
					}
					dw.Close()
				} else {
					w.PrintfLine("500 error, %s", err.Error())
				}
			}
		} else if strings.HasPrefix(lline, "article ") {
			if newsgroup == "" {
				w.PrintfLine("412 No Newsgroup Selected")
			} else {
				article := line[8:]
				var msgid string
				var article_no int64
				if ValidMessageID(article) {
					article_no = 0 // eh
				} else {
					article_no, err = strconv.ParseInt(article, 10, 32)
					if err == nil {
						msgid, err = self.db.GetMessageIDForNNTPID(newsgroup, article_no)
					}
				}
				if err == nil {
					w.PrintfLine("220 %d %s", article_no, msgid)
					fname := self.store.GetFilename(msgid)
					f, err := os.Open(fname)
					if err == nil {
						dw := w.DotWriter()
						_, err = io.Copy(dw, f)
						dw.Close()
						f.Close()
					}
				} else {
					w.PrintfLine("500 error, %s", err.Error())
				}
			}
		} else if lline == "mode reader" {
			w.PrintfLine("201 posting disallowed")
		} else if strings.HasPrefix(lline, "mode ") {
			// handle other mode
			w.PrintfLine("%d mode not implemented", 501)
		} else if lline == "capabilities" {
			// send capabilities
			dw := w.DotWriter()
			io.WriteString(dw, "101 yeh we can do stuff\r\n")
			io.WriteString(dw, "VERSION 2\r\n")
			io.WriteString(dw, "IMPLEMENTATION srndv2 nntp frontend\r\n")
			io.WriteString(dw, "READER\r\n")
			dw.Close()
		} else {
			// idk what command this is, log it and report error
			log.Println("invalid line from nntp frontend connection:", line)
			w.PrintfLine("500 idk what that means")
		}
	}
	sock.Close()
	log.Println("nntp frontend connection closed")
}
