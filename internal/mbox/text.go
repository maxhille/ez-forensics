package mbox

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxTextBody caps extracted text at 5 MB to bound memory per message.
const maxTextBody = 5 << 20

// ExtractText returns the text content of a message, preferring text/plain
// over text/html. Attachments and other parts are skipped.
func ExtractText(msg *mail.Message) (string, error) {
	ct := msg.Header.Get("Content-Type")
	if ct == "" {
		return decodeText(msg.Header.Get("Content-Transfer-Encoding"), "text/plain", "us-ascii", msg.Body)
	}

	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return "", fmt.Errorf("parse content-type: %w", err)
	}

	switch {
	case strings.HasPrefix(mt, "multipart/"):
		boundary := params["boundary"]
		if boundary == "" {
			return "", errors.New("multipart without boundary")
		}
		reader := multipart.NewReader(msg.Body, boundary)
		var html string
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				if html != "" {
					return html, nil
				}
				return "", err
			}
			text, err := ExtractText(&mail.Message{
				Header: mail.Header(part.Header),
				Body:   part,
			})
			part.Close()
			if err != nil {
				continue
			}
			if strings.Contains(part.Header.Get("Content-Type"), "text/html") {
				if text != "" {
					html = text
				}
				continue
			}
			if text != "" {
				return text, nil
			}
		}
		return html, nil

	case mt == "text/plain" || mt == "text/html":
		return decodeText(msg.Header.Get("Content-Transfer-Encoding"), mt, params["charset"], msg.Body)

	default:
		return "", nil
	}
}

// decodeText applies transfer decoding and charset conversion, strips HTML
// markup for text/html parts, and returns UTF-8 text.
func decodeText(transfer, mt, charset string, r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxTextBody))
	if err != nil {
		return "", err
	}

	data, err = decodeTransfer(data, transfer)
	if err != nil {
		return "", err
	}
	if charset != "" {
		if decoded, err := toUTF8(data, strings.ToLower(charset)); err == nil {
			data = decoded
		}
	}
	if mt == "text/html" {
		data = stripHTML(data)
	}
	return strings.TrimSpace(string(data)), nil
}

// decodeTransfer decodes base64 and quoted-printable bodies.
func decodeTransfer(data []byte, transfer string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(strings.NewReader(string(data))))
	case "base64":
		clean := make([]byte, 0, len(data))
		for _, b := range data {
			switch b {
			case '\r', '\n', ' ', '\t':
			default:
				clean = append(clean, b)
			}
		}
		return base64.StdEncoding.DecodeString(string(clean))
	default: // 7bit, 8bit, binary, empty
		return data, nil
	}
}

// toUTF8 converts legacy single-byte charsets to UTF-8. Charsets that are
// already UTF-8 or ASCII-compatible are returned as-is.
func toUTF8(data []byte, charset string) ([]byte, error) {
	switch charset {
	case "utf-8", "us-ascii", "ascii", "":
		if utf8.Valid(data) {
			return data, nil
		}
	case "iso-8859-1", "latin1", "iso8859-1", "windows-1252", "cp1252":
		out := make([]rune, len(data))
		for i, b := range data {
			out[i] = rune(b)
		}
		return []byte(string(out)), nil
	case "iso-8859-15", "latin9":
		out := make([]rune, len(data))
		for i, b := range data {
			switch b {
			case 0xA4:
				out[i] = '€'
			case 0xA6:
				out[i] = 'Š'
			case 0xA8:
				out[i] = 'š'
			case 0xB4:
				out[i] = 'ž'
			case 0xB8:
				out[i] = 'Ž'
			case 0xBC:
				out[i] = 'Œ'
			case 0xBD:
				out[i] = 'œ'
			case 0xBE:
				out[i] = 'Ÿ'
			default:
				out[i] = rune(b)
			}
		}
		return []byte(string(out)), nil
	}
	if utf8.Valid(data) {
		return data, nil
	}
	return nil, errors.New("unsupported charset: " + charset)
}

// stripHTML removes HTML tags, script and style blocks, and entities,
// producing readable plain text.
func stripHTML(data []byte) []byte {
	s := string(data)
	for _, tag := range []string{"script", "style", "head"} {
		for {
			start := findTag(s, tag)
			if start < 0 {
				break
			}
			end := strings.Index(strings.ToLower(s[start:]), "</"+tag)
			if end < 0 {
				s = s[:start]
				break
			}
			s = s[:start] + s[start+end+len(tag)+3:]
		}
	}
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = decodeEntities(s)
	s = strings.NewReplacer("\r", "", "\n{3,}", "\n\n").Replace(s)
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return []byte(strings.TrimSpace(s))
}

var htmlTagRe = regexp.MustCompile(`(?is)<[^>]*>`)

func findTag(s, tag string) int {
	lower := strings.ToLower(s)
	needle := "<" + tag
	for i := 0; i+len(needle) <= len(lower); {
		idx := strings.Index(lower[i:], needle)
		if idx < 0 {
			return -1
		}
		pos := i + idx
		rest := lower[pos+len(needle):]
		if len(rest) == 0 {
			return -1
		}
		if rest[0] == '>' || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n' {
			return pos
		}
		i = pos + len(needle)
	}
	return -1
}

// decodeEntities decodes common HTML entities to plain text.
func decodeEntities(s string) string {
	replacer := strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", "\"", "&#39;", "'", "&apos;", "'", "&hellip;", "…",
		"&mdash;", "—", "&ndash;", "–", "&auml;", "ä", "&ouml;", "ö",
		"&uuml;", "ü", "&Auml;", "Ä", "&Ouml;", "Ö", "&Uuml;", "Ü",
		"&szlig;", "ß", "&eacute;", "é", "&egrave;", "è",
	)
	return replacer.Replace(s)
}
