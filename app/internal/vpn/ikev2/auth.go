package ikev2

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
)

// AUTH の方式 (RFC 7296 3.8、RFC 4754、RFC 7427)
const (
	authRSA        = 1
	authPSK        = 2
	authECDSA256   = 9
	authECDSA384   = 10
	authECDSA521   = 11
	authDigitalSig = 14
)

const certX509 = 4

// pskAuth は共有の鍵 (EAP では MSK) で作る AUTH の値 (RFC 7296 2.15)
func pskAuth(s *suite, secret, signed []byte) []byte {
	return s.prfFunc(s.prfFunc(secret, []byte("Key Pad for IKEv2")), signed)
}

// verifyServerCert は、サーバーの証明書を CA までたどり、つないだ名前と合うかを確かめる
func verifyServerCert(certs [][]byte, roots *x509.CertPool, host string) (*x509.Certificate, error) {
	if len(certs) == 0 {
		return nil, errors.New("VPN のサーバーが証明書を送ってきません")
	}
	var parsed []*x509.Certificate
	for _, c := range certs {
		cert, err := x509.ParseCertificate(c)
		if err != nil {
			return nil, fmt.Errorf("VPN のサーバーの証明書を読めません: %w", err)
		}
		parsed = append(parsed, cert)
	}
	inter := x509.NewCertPool()
	for _, c := range parsed[1:] {
		inter.AddCert(c)
	}
	_, err := parsed[0].Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil {
		return nil, fmt.Errorf("VPN のサーバーの証明書を確かめられません: %w", err)
	}
	return parsed[0], nil
}

var (
	oidSHA1RSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 5}
	oidSHA256RSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidSHA384RSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	oidSHA512RSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
	oidRSAPSS    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 10}
	oidECDSA1    = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 1}
	oidECDSA256  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidECDSA384  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidECDSA512  = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
	oidEd25519   = asn1.ObjectIdentifier{1, 3, 101, 112}
	oidSHA1      = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSHA256    = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA384    = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512    = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
)

func hashOf(oid asn1.ObjectIdentifier) crypto.Hash {
	switch {
	case oid.Equal(oidSHA1):
		return crypto.SHA1
	case oid.Equal(oidSHA256):
		return crypto.SHA256
	case oid.Equal(oidSHA384):
		return crypto.SHA384
	case oid.Equal(oidSHA512):
		return crypto.SHA512
	}
	return 0
}

func digest(h crypto.Hash, data []byte) []byte {
	switch h {
	case crypto.SHA1:
		d := sha1.Sum(data)
		return d[:]
	case crypto.SHA256:
		d := sha256.Sum256(data)
		return d[:]
	case crypto.SHA384:
		d := sha512.Sum384(data)
		return d[:]
	case crypto.SHA512:
		d := sha512.Sum512(data)
		return d[:]
	}
	return nil
}

type pssParams struct {
	Hash pkix.AlgorithmIdentifier `asn1:"optional,explicit,tag:0"`
	MGF  pkix.AlgorithmIdentifier `asn1:"optional,explicit,tag:1"`
	Salt int                      `asn1:"optional,explicit,tag:2,default:20"`
}

var errSignature = errors.New("VPN のサーバーの署名が合いません")

// verifySignature は、サーバーの AUTH の署名を、証明書の公開鍵で確かめる
func verifySignature(method byte, data []byte, cert *x509.Certificate, signed []byte) error {
	pub := cert.PublicKey
	switch method {
	case authRSA:
		rk, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errSignature
		}
		// ハッシュは決まっていないので、署名の中の DigestInfo に合うものを探す
		for _, h := range []crypto.Hash{crypto.SHA1, crypto.SHA256, crypto.SHA384, crypto.SHA512} {
			if rsa.VerifyPKCS1v15(rk, h, digest(h, signed), data) == nil {
				return nil
			}
		}
		return errSignature
	case authECDSA256, authECDSA384, authECDSA521:
		ek, ok := pub.(*ecdsa.PublicKey)
		if !ok || len(data)%2 != 0 {
			return errSignature
		}
		h := map[byte]crypto.Hash{authECDSA256: crypto.SHA256, authECDSA384: crypto.SHA384, authECDSA521: crypto.SHA512}[method]
		n := len(data) / 2
		r, s := new(big.Int).SetBytes(data[:n]), new(big.Int).SetBytes(data[n:])
		if !ecdsa.Verify(ek, digest(h, signed), r, s) {
			return errSignature
		}
		return nil
	case authDigitalSig:
		if len(data) < 1 || len(data) < 1+int(data[0]) {
			return errMalformed
		}
		var alg pkix.AlgorithmIdentifier
		if _, err := asn1.Unmarshal(data[1:1+int(data[0])], &alg); err != nil {
			return errMalformed
		}
		sig := data[1+int(data[0]):]
		return verifyAlg(alg, sig, pub, signed)
	}
	return fmt.Errorf("AUTH の方式 %d には対応していません", method)
}

func verifyAlg(alg pkix.AlgorithmIdentifier, sig []byte, pub any, signed []byte) error {
	oid := alg.Algorithm
	if oid.Equal(oidEd25519) {
		ek, ok := pub.(ed25519.PublicKey)
		if !ok || !ed25519.Verify(ek, signed, sig) {
			return errSignature
		}
		return nil
	}
	rsaHash := map[string]crypto.Hash{oidSHA1RSA.String(): crypto.SHA1, oidSHA256RSA.String(): crypto.SHA256, oidSHA384RSA.String(): crypto.SHA384, oidSHA512RSA.String(): crypto.SHA512}
	ecHash := map[string]crypto.Hash{oidECDSA1.String(): crypto.SHA1, oidECDSA256.String(): crypto.SHA256, oidECDSA384.String(): crypto.SHA384, oidECDSA512.String(): crypto.SHA512}
	if h, ok := rsaHash[oid.String()]; ok {
		rk, ok := pub.(*rsa.PublicKey)
		if !ok || rsa.VerifyPKCS1v15(rk, h, digest(h, signed), sig) != nil {
			return errSignature
		}
		return nil
	}
	if h, ok := ecHash[oid.String()]; ok {
		ek, ok := pub.(*ecdsa.PublicKey)
		if !ok || !ecdsa.VerifyASN1(ek, digest(h, signed), sig) {
			return errSignature
		}
		return nil
	}
	if oid.Equal(oidRSAPSS) {
		var p pssParams
		if _, err := asn1.Unmarshal(alg.Parameters.FullBytes, &p); err != nil {
			return errMalformed
		}
		h := crypto.SHA1
		if len(p.Hash.Algorithm) > 0 {
			h = hashOf(p.Hash.Algorithm)
		}
		rk, ok := pub.(*rsa.PublicKey)
		if !ok || h == 0 {
			return errSignature
		}
		if rsa.VerifyPSS(rk, h, digest(h, signed), sig, &rsa.PSSOptions{SaltLength: p.Salt, Hash: h}) != nil {
			return errSignature
		}
		return nil
	}
	return fmt.Errorf("署名の方式 %s には対応していません", oid)
}
