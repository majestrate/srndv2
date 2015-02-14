//
// nntp.go
//

type NNTPConnection struct {
	reader *bufio.Reader
	writer *bufio.Writer
}

