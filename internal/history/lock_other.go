//go:build !unix

package history

import "os"

func withLock(f *os.File, fn func() error) error {
	return fn()
}
