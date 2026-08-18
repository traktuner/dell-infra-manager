package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sync"
)

// Version and Commit are set by release builds through -ldflags. Local builds
// keep explicit development values and therefore never pretend to be a release.
var (
	Version = "dev"
	Commit  = "unknown"
)

var (
	hashOnce sync.Once
	hash     string
	hashErr  error
)

func BinarySHA256() (string, error) {
	hashOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			hashErr = err
			return
		}
		f, err := os.Open(path)
		if err != nil {
			hashErr = err
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			hashErr = err
			return
		}
		hash = hex.EncodeToString(h.Sum(nil))
	})
	return hash, hashErr
}
