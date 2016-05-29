package nntp

import (
	"errors"
)

// authentication was rejected
var ErrAuthRejected = errors.New("authentication rejected")

// defines type for nntp authentication mechanism on client side
type ClientAuth interface {
	// send authenticate to server connected via established nntp connection
	// returns nil on success otherwise an error
	// retirns ErrAuthRejected if the authentication was rejected
	SendAuth(c Conn) error
}

// defines server side authentication mechanism
type ServerAuth interface {
	// handle authentication phase with an established nntp connection
	// returns nil on success otherwise error if one occurs during authentication
	// returns ErrAuthRejected if we rejected the authentication
	HandleAuth(c Conn) error
}
