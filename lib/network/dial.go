package network

import (
	"errors"
	"net"
)

// operation timed out
var ErrTimeout = errors.New("timeout")

// the operation was reset abruptly
var ErrReset = errors.New("reset")

// the operation was actively refused
var ErrRefused = errors.New("refused")

// generic dialer
// dials out to a remote address
// returns a net.Conn and nil on success
// returns nil and error if an error happens while dialing
type Dialer interface {
	Dial(remote net.Addr) (net.Conn, error)
}
