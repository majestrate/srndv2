package model

import (
	"testing"
)

func TestGenMessageID(t *testing.T) {
	msgid := GenMessageID("test.tld")
	t.Logf("generated id %s", msgid)
	if !msgid.Valid() {
		t.Logf("invalid generated message-id %s", msgid)
		t.Fail()
	}
	msgid = GenMessageID("<><><>")
	t.Logf("generated id %s", msgid)
	if msgid.Valid() {
		t.Logf("generated valid message-id when it should've been invalid %s", msgid)
		t.Fail()
	}
}

func TestMessageIDHash(t *testing.T) {
	msgid := GenMessageID("test.tld")
	lh := msgid.LongHash()
	sh := msgid.ShortHash()
	bh := msgid.Blake2Hash()
	t.Logf("long=%s short=%s blake2=%s", lh, sh, bh)
}
