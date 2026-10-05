package main

import (
	"errors"
)

var errFailWriter = errors.New("write failed")

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errFailWriter
}
