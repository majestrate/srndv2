package nntp

import (
	"crypto/tls"
	"encoding/json"
	"net"
	"net/textproto"
)

// state of an nntp connection
type ConnState struct {
	// name of parent feed
	FeedName string `json:"feedname"`
	// name of the connection
	ConnName string `json:"connname"`
	// hostname of remote connection
	HostName string `json:"hostname"`
	// current nntp mode
	Mode string `json:"mode"`
	// current selected nntp newsgroup
	Group string `json:"newsgroup"`
	// current selected nntp article
	Article string `json:"article"`
	// parent feed's policy
	Policy *FeedPolicy `json:"feedpolicy"`
}

// nntp connection to remote server
type Conn struct {
	// buffered connection
	C *textproto.Conn
	// remote address
	Addr net.Addr

	// unexported fields ...

	// connection state (mutable)
	state ConnState
	// state of tls connection
	tlsState *tls.ConnectionState
	// has this connection authenticated yet?
	authenticated bool
	// the username logged in with if it has authenticated via user/pass
	username string
	// underlying network socket
	conn net.Conn
}

// json representation of this connection
// format is:
// {
//   "state" : (connection state object),
//   "authed" : bool,
//   "tls" : (tls info or null if plaintext connection)
// }
func (c *Conn) MarshalJSON() ([]byte, error) {
	j := make(map[string]interface{})
	j["state"] = c.state
	j["authed"] = c.authenticated
	j["tls"] = c.tlsState
	return json.Marshal(j)
}

// get the current state of our connection (immutable)
func (c *Conn) GetState() (state *ConnState) {
	return &ConnState{
		FeedName: c.state.FeedName,
		ConnName: c.state.ConnName,
		HostName: c.state.HostName,
		Mode:     c.state.Mode,
		Group:    c.state.Group,
		Article:  c.state.Article,
		Policy: &FeedPolicy{
			Whitelist:            c.state.Policy.Whitelist,
			Blacklist:            c.state.Policy.Blacklist,
			AllowAnonPosts:       c.state.Policy.AllowAnonPosts,
			AllowAnonAttachments: c.state.Policy.AllowAnonAttachments,
			AllowAttachments:     c.state.Policy.AllowAttachments,
			UntrustedRequiresPoW: c.state.Policy.UntrustedRequiresPoW,
		},
	}
}
