//go:build unix

package history

import (
	"os"
	"syscall"
	"time"
)

const lockWait = 200 * time.Millisecond

func withLock(f *os.File, fn func() error) error {
	deadline := time.Now().Add(lockWait)
	var last error
	for {
		last = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if last == nil {
			break
		}
		if time.Now().After(deadline) {
			return last
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
