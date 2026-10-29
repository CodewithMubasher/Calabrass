//go:build windows

package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// platformKeyStore protects the master key with DPAPI. The blob on disk can
// only be opened by the same Windows user on the same machine, which is the
// protection Windows gives its own saved credentials.
func platformKeyStore(dataDir string) KeyStore {
	return dpapiStore{path: filepath.Join(dataDir, "calabrass.key.dpapi")}
}

type dpapiStore struct{ path string }

func (dpapiStore) Name() string { return "Windows DPAPI (current user)" }

var (
	crypt32       = syscall.NewLazyDLL("crypt32.dll")
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	procProtect   = crypt32.NewProc("CryptProtectData")
	procUnprotect = crypt32.NewProc("CryptUnprotectData")
	procLocalFree = kernel32.NewProc("LocalFree")
)

const cryptProtectUIForbidden = 0x1

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(data []byte) *dataBlob {
	if len(data) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
}

func (b *dataBlob) bytes() []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

func (d dpapiStore) Get() ([]byte, error) {
	blob, err := os.ReadFile(d.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	in := newBlob(blob)
	var out dataBlob
	r, _, callErr := procUnprotect.Call(
		uintptr(unsafe.Pointer(in)), 0, 0, 0, 0, cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, errors.New("Windows could not unlock the key for this user: " + callErr.Error())
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return out.bytes(), nil
}

func (d dpapiStore) Put(key []byte) error {
	in := newBlob(key)
	var out dataBlob
	r, _, callErr := procProtect.Call(
		uintptr(unsafe.Pointer(in)), 0, 0, 0, 0, cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return errors.New("Windows could not protect the key: " + callErr.Error())
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	if err := os.MkdirAll(filepath.Dir(d.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(d.path, out.bytes(), 0o600)
}
