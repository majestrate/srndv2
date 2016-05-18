package message

import (
	"bufio"
	"io"
	"strings"
)

// an nntp message header
type Header map[string][]string

// do we have a key in this header?
func (self Header) Has(key string) bool {
	_, ok := self[key]
	return ok
}

// set key value
func (self Header) Set(key, val string) {
	self[key] = []string{val}
}

// append value to key
func (self Header) Add(key, val string) {
	if self.Has(key) {
		self[key] = append(self[key], val)
	} else {
		self.Set(key, val)
	}
}

// get via key or return fallback value
func (self Header) Get(key, fallback string) string {
	val, ok := self[key]
	if ok {
		str := ""
		for _, k := range val {
			str += k + ", "
		}
		return str[:len(str)-2]
	} else {
		return fallback
	}
}

// interface for types that can read an nntp header
type HeaderReader interface {
	// blocking read an nntp header from an io.Reader
	// return the read header and nil on success
	// return nil and an error if an error occurred while reading
	ReadHeader(r io.Reader) (Header, error)
}

// interface for types that can write an nntp header
type HeaderWriter interface {
	// blocking write an nntp header to an io.Writer
	// returns an error if one occurs otherwise nil
	WriteHeader(hdr Header, w io.Writer) error
}

// implements HeaderReader and HeaderWriter
type HeaderIO struct {
	delim byte
}

// read header
func (s *HeaderIO) ReadHeader(r io.Reader) (hdr Header, err error) {
	hdr = make(Header)
	br := bufio.NewReader(r)
	var line string
	for err == nil {
		line, err = br.ReadString(s.delim)
		if err == nil {
			// strip out line endings
			line = strings.Trim(line, "\r\n")
			if len(line) > 0 {
				// split it via :
				idx := strings.Index(line, ": ")
				if idx > 0 {
					// valid line
					k := line[:idx]
					v := line[2+idx:]
					hdr.Add(k, v)
				}
				// invalid lines are ingored
			} else {
				// end of header
				break
			}
		}
	}
	if err != nil {
		hdr = nil
	}
	return
}

// write header
func (s *HeaderIO) WriteHeader(hdr Header, wr io.Writer) (err error) {
	for k, vs := range hdr {
		for _, v := range vs {
			var line []byte
			// key
			line = append(line, []byte(k)...)
			// ": "
			line = append(line, 32, 58)
			// value
			line = append(line, []byte(v)...)
			// delimiter
			line = append(line, s.delim)
			// write line
			_, err = wr.Write(line)
			if err != nil {
				return
			}
		}
	}
	return
}

func NewHeaderIO() *HeaderIO {
	return &HeaderIO{
		delim: 10,
	}
}
