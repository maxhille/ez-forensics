package api

import (
	"fmt"
	"io"
	"os"
)

// readRaw reads size bytes at offset from an mbox file. head limits how many
// bytes are returned. Raw bytes come straight from the evidence file.
func readRaw(path string, offset, size, head int64) ([]byte, error) {
	if head <= 0 || head > size {
		head = size
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %s@%d: %w", path, offset, err)
	}
	buf := make([]byte, head)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}
