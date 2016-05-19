package nntp

// an nntp connection
type Conn interface {

	// negotiate an nntp session on this connection
	// returns nil if we negitated successfully
	// returns ErrAuthRejected if the remote server rejected any authentication
	// we sent or another error if one occured while negotiating
	Negotiate() error

	// obtain connection state
	GetState() *ConnState

	// retutrn true if posting is allowed
	// return false if posting is not allowed
	PostingAllowed() bool

	// handle inbound non-streaming connection
	// apply filters sequentially for each article, then call event hooks
	ProcessInbound(filters []ArticleFilter, hooks EventHooks)

	// does this connection want to do nntp streaming?
	WantsStreaming() bool

	// what mode are we in?
	// returns mode in all caps
	Mode() Mode

	// initiate nntp streaming
	// after calling this the caller MUST call StreamAndQuit()
	// returns a channel for message ids, true if caller sends on the channel or
	// false if the channel is for the caller to recv with
	// returns nil and ErrStreamingNotAllowed if streaming is not allowed on this
	// connection or another error if one occurs while trying to start streaming
	StartStreaming() (chan ArticleEntry, bool, error)

	// stream articles and quit when the channel obtained by StartStreaming() is
	// closed, after which this nntp connection is no longer open
	// check inbound articles with given ArticlePolicy, if article is accepted
	// apply filters sequentially for each article, then call event hooks
	StreamAndQuit(acceptor ArticleAcceptor, filters []ArticleFilter, hooks EventHooks)

	// is this nntp connection open?
	IsOpen() bool

	// send quit command and close connection
	Quit()
}
