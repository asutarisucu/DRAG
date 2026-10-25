//go:build !windows

package store

import "errors"

// PC アプリは Windows だけで動かす。ほかの OS では VPN の鍵を保存しない
func protect([]byte) ([]byte, error) {
	return nil, errors.ErrUnsupported
}

func unprotect([]byte) ([]byte, error) {
	return nil, errors.ErrUnsupported
}
