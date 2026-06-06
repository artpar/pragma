package session

import (
	"bufio"
	"io"
)

const (
	jsonlInitialBufferSize = 1 << 20
	jsonlMaxLineSize       = 64 << 20 // file_state entries can include cached file contents
)

func newJSONLScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, jsonlInitialBufferSize), jsonlMaxLineSize)
	return scanner
}
