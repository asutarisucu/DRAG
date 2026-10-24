// Package secret は、API のトークンのような秘密を、この PC のこの利用者だけが読める形にする。
// Windows では DPAPI (CryptProtectData) を使う。x/sys@v0.49.0 windows/zsyscall_windows.go の CryptProtectData と CryptUnprotectData
package secret

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protect は data を暗号化する。同じ Windows の利用者でしか戻せない
func Protect(data []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(data), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return take(&out), nil
}

// Unprotect は Protect したものを戻す
func Unprotect(data []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(data), nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return take(&out), nil
}

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// take は DPAPI が確保した領域を写して放す
func take(b *windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	return append([]byte(nil), unsafe.Slice(b.Data, b.Size)...)
}
