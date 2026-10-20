// Package mbox splits mbox files into single messages without loading the
// whole file into memory. Each message is reported with its byte offset and
// size inside the file, so raw contents stay in the original evidence file.
package mbox

import (
	"bufio"
	"io"
	"net/mail"
	"strings"
)

// Message is one parsed email from an mbox file.
type Message struct {
	Offset int64 // byte offset of the "From " line in the file
	Size   int64 // byte size of the whole message including the "From " line
	Header mail.Header
	Body   string // extracted text (text/plain preferred over text/html)
}

// isFromLine reports whether the line is an mbox "From " separator.
func isFromLine(line string) bool {
	return strings.HasPrefix(line, "From ") || line == "From"
}

// Parse reads an mbox file and calls fn for every message. Messages are
// split on lines starting with "From " that directly follow a blank line
// (or start of file), per the mbox convention. Lines in the message body
// that were escaped as ">From " are unescaped for header/body parsing.
func Parse(r io.Reader, fn func(*Message) error) error {
	reader := bufio.NewReaderSize(r, 1<<20)

	var (
		lines    []string // raw lines of the current message
		offset   int64    // offset of current message start
		pos      int64    // current read position
		prevLine string
		inMsg    bool
	)

	flush := func() error {
		if !inMsg {
			return nil
		}
		size := pos - offset
		msg, err := parseMessage(lines, offset, size)
		if err != nil {
			return err
		}
		return fn(msg)
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if inMsg && prevLine == "" && isFromLine(trimmed) {
				if err := flush(); err != nil {
					return err
				}
				lines = lines[:0]
				offset = pos
				inMsg = true
			} else if !inMsg {
				if isFromLine(trimmed) {
					offset = pos
					inMsg = true
				}
			}
			if inMsg {
				lines = append(lines, trimmed)
			}
			prevLine = trimmed
			pos += int64(len(line))
		}
		if err == io.EOF {
			if err := flush(); err != nil {
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// parseMessage reassembles the raw lines and parses headers and text body.
func parseMessage(lines []string, offset, size int64) (*Message, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	// Drop the "From " line, unescape ">From " lines.
	raw := make([]byte, 0, size)
	for i, line := range lines[1:] {
		if i > 0 {
			raw = append(raw, '\n')
		}
		if strings.HasPrefix(line, ">From") {
			raw = append(raw, line[1:]...)
		} else {
			raw = append(raw, line...)
		}
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		// Keep unparseable messages with empty headers instead of failing
		// the whole import.
		return &Message{Offset: offset, Size: size, Header: mail.Header{}}, nil
	}
	body, err := ExtractText(msg)
	if err != nil {
		body = ""
	}
	return &Message{
		Offset: offset,
		Size:   size,
		Header: msg.Header,
		Body:   body,
	}, nil
}
