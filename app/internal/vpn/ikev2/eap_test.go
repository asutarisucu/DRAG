package ikev2

import (
	"encoding/hex"
	"strings"
	"testing"
)

func unhex(s string) []byte {
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		panic(err)
	}
	return b
}

// RFC 2759 9.2 の値
func TestMSCHAPv2HashExample(t *testing.T) {
	auth := unhex("5B 5D 7C 7D 7B 3F 2F 3E 3C 2C 60 21 32 26 26 28")
	peer := unhex("21 40 23 24 25 5E 26 2A 28 29 5F 2B 3A 33 7C 7E")
	if got := hex.EncodeToString(challengeHash(peer, auth, "User")); got != "d02e4386bce91226" {
		t.Fatalf("ChallengeHash %s", got)
	}
	if got := hex.EncodeToString(ntPasswordHash("clientPass")); got != "44ebba8d5312b8d611474411f56989ae" {
		t.Fatalf("NtPasswordHash %s", got)
	}
	nt := ntResponse(auth, peer, "User", "clientPass")
	if got := hex.EncodeToString(nt); got != "82309ecd8d708b5ea08faa3981cd83544233114a3d85d6df" {
		t.Fatalf("NT-Response %s", got)
	}
	if got := authenticatorResponse("clientPass", nt, peer, auth, "User"); got != "S=407A5589115FD0D6209F510FE9C04566932CDA56" {
		t.Fatalf("AuthenticatorResponse %s", got)
	}
	// ドメインを付けても、ChallengeHash にはユーザー名だけを使う
	if hex.EncodeToString(challengeHash(peer, auth, `BIGCO\User`)) != "d02e4386bce91226" {
		t.Fatal("ドメインを外していない")
	}
}
