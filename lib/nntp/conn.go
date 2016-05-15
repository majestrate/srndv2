package nntp

import (
	"net"
	"net/textproto"
	"sync"
)

// state of an nntp connection
type ConnState struct {
	// name of parent feed
	FeedName string
	// name of the connection
	ConnName string
	// hostname of remote connection
	HostName string
	// current nntp mode
	Mode string
	// current selected nntp newsgroup
	Group string
	// current selected nntp article
	Article string
	// parent feed's policy
	Policy FeedPolicy
}

// nntp connection to remote server
type Conn struct {
	// buffered connection
	C *textproto.Conn
	// underlying socket
	NetC net.Conn

	// unexported fields ...

	// connection state (mutable)
	state ConnState
	// mutex for accessing underlying io when pipelining
	access sync.Mutex
	// channel to send ARTICLE <msgid> commands down
	articleChnl chan MessageID
}

// get the current state of our connection (immutable)
func (c *Conn) GetState() (state *ConnState) {
	return
}
