package secret

import (
	"bytes"
	"runtime"
	"testing"
)

func TestProtectRoundTrip(t *testing.T) {
	in := []byte("cf-token-for-test")
	enc, err := Protect(in)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && bytes.Contains(enc, in) {
		t.Fatal("暗号化されていない")
	}
	out, err := Unprotect(enc)
	if err != nil || !bytes.Equal(out, in) {
		t.Fatalf("%q %v", out, err)
	}
}
