//go:build !windows

package secret

// Windows のほかでは暗号化しない。ファイルは持ち主だけが読める権限で置く

func Protect(data []byte) ([]byte, error) { return data, nil }

func Unprotect(data []byte) ([]byte, error) { return data, nil }
