// SPDX-License-Identifier: GPL-3.0-or-later

package device

import (
	"os"

	"golang.org/x/sys/unix"
)

// Watcher signals changes in /dev/input: a device node appearing, or its
// permissions changing (udev applies ACLs just after creating the node).
type Watcher struct {
	f *os.File
	C <-chan struct{}
}

// Watch starts watching /dev/input. Close stops it.
func Watch() (*Watcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(fd, "/dev/input", unix.IN_CREATE|unix.IN_ATTRIB); err != nil {
		unix.Close(fd)
		return nil, err
	}
	// A non-blocking fd lets the runtime poller unblock Read on Close.
	f := os.NewFile(uintptr(fd), "inotify")
	c := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := f.Read(buf); err != nil {
				close(c)
				return
			}
			select {
			case c <- struct{}{}:
			default: // a change is already pending
			}
		}
	}()
	return &Watcher{f: f, C: c}, nil
}

// Close stops the watcher.
func (w *Watcher) Close() error { return w.f.Close() }
