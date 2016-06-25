package model

import (
	"time"
)

type ArticleHeader map[string][]string

// a ( time point, post count ) tuple
type PostEntry [2]int64

func (self PostEntry) Time() time.Time {
	return time.Unix(self[0], 0)
}

func (self PostEntry) Count() int64 {
	return self[1]
}
