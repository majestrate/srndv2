package srnd

import "testing"
import "github.com/majestrate/nacl"

func TestSignVerify(t *testing.T) {

	msgid := "<asd@asd.asd>"
	secret := "asdasdasd"
	seed := parseTripcodeSecret(secret)
	kp := nacl.LoadSignKey(seed)
	defer kp.Free()
	pubkey := hexify(kp.Public())
	seckey := kp.Secret()
	sig := msgidFrontendSign(seckey, msgid)
	if !verifyFrontendSig(pubkey, sig, msgid) {
		t.Fail()
	}
}
