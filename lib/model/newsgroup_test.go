package model

import (
	"testing"
)

func TestValidNewsgroup(t *testing.T) {
	g := Newsgroup("overchan.test")
	if !g.Valid() {
		t.Logf("%s is invalid?", g)
		t.Fail()
	}
}

func TestInvalidNewsgroup(t *testing.T) {
	g := Newsgroup("asd.asd.asd.&&&")
	if g.Valid() {
		t.Logf("%s should be invalid", g)
		t.Fail()
	}

}
