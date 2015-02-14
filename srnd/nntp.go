//
// nntp.go
//
package srnd
import (
	"bufio"
)

type NNTPConnection struct {
	reader *bufio.Reader
	writer *bufio.Writer
}

