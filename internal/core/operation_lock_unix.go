//go:build !windows

package core

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockOperationFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrOperationBusy
	}
	return err
}
