package store

import (
	"os"
	"sync"
	"syscall"
)

// lockSlug takes an exclusive advisory lock on the store root and returns the
// function that releases it. flock works across processes (the CLI and the
// server share a store) and between separate opens in one process, so it
// serializes every writer of a preview's directory and manifest. The lock is on
// the root directory itself, which leaves no lock files behind; only the short
// manifest-read-and-swap section is held under it, never the file copying. The
// returned function is safe to call more than once.
func (s *Store) lockSlug(string) (func(), error) {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Open(s.Root)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			f.Close()
		})
	}, nil
}
