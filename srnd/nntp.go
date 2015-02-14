//
// nntp.go
//
package main

import (
	"bufio"
)

type NNTPConnection struct {
	reader *bufio.Reader
	writer *bufio.Writer
}

