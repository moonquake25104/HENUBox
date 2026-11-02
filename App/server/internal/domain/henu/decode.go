package henu

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// FileKind classifies the uploaded document by its actual bytes, never by the
// file extension.
type FileKind string

const (
	KindHTML    FileKind = "html"
	KindOLE     FileKind = "ole_xls"
	KindZIP     FileKind = "zip_xlsx"
	KindUnknown FileKind = "unknown"
)

var (
	// ErrUnsupportedFileType is returned for binary XLS/XLSX containers; the
	// HTTP layer maps it to 415 UNSUPPORTED_FILE_TYPE.
	ErrUnsupportedFileType = errors.New("unsupported file type")
	// ErrUndecodable is returned when the bytes are neither valid UTF-8 nor
	// valid GBK/GB18030; the HTTP layer maps it to 422 UNRECOGNIZED_DOCUMENT.
	ErrUndecodable = errors.New("document encoding is not supported")
)

var (
	oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}
	// charset=... appears in the <meta> tag; the tag itself is ASCII even when
	// the rest of the document is GBK, so scanning raw bytes is safe.
	charsetRe = regexp.MustCompile(`(?i)charset\s*=\s*["']?\s*([a-z0-9_\-]+)`)
)

// DetectKind classifies raw bytes. HTML is not assumed: a document must look
// like markup to be accepted.
func DetectKind(raw []byte) FileKind {
	head := raw
	if len(head) > 4096 {
		head = head[:4096]
	}
	switch {
	case bytes.HasPrefix(raw, oleMagic):
		return KindOLE
	case bytes.HasPrefix(raw, zipMagic):
		return KindZIP
	}
	lower := bytes.ToLower(head)
	if bytes.Contains(lower, []byte("<html")) ||
		bytes.Contains(lower, []byte("<table")) ||
		bytes.Contains(lower, []byte("<!doctype html")) {
		return KindHTML
	}
	return KindUnknown
}

// Decode turns the raw upload into text. It decodes strictly: a byte stream
// that produces replacement characters is rejected instead of silently
// corrupting course names.
func Decode(raw []byte) (string, error) {
	switch DetectKind(raw) {
	case KindOLE, KindZIP:
		return "", ErrUnsupportedFileType
	}

	text, err := decodeText(raw)
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(text, '\uFFFD') {
		return "", fmt.Errorf("%w: replacement character produced", ErrUndecodable)
	}
	return text, nil
}

func decodeText(raw []byte) (string, error) {
	switch {
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		body := raw[3:]
		if !utf8.Valid(body) {
			return "", fmt.Errorf("%w: invalid utf-8 after BOM", ErrUndecodable)
		}
		return string(body), nil
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return decodeWith(raw[2:], unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder())
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return decodeWith(raw[2:], unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder())
	}

	label := declaredCharset(raw)
	switch normalizeCharset(label) {
	case "gbk", "gb2312", "gb18030", "cp936":
		return decodeGB(raw)
	case "utf-8", "utf8":
		if !utf8.Valid(raw) {
			return "", fmt.Errorf("%w: declared utf-8 but bytes are invalid", ErrUndecodable)
		}
		return string(raw), nil
	}
	if label != "" {
		// A declared but unsupported charset must not be silently decoded with a
		// different one: that turns a wrong guess into wrong course names.
		return "", fmt.Errorf("%w: unsupported declared charset %q", ErrUndecodable, label)
	}

	// No usable declaration: prefer UTF-8 (strict), then GB18030.
	if utf8.Valid(raw) {
		return string(raw), nil
	}
	return decodeGB(raw)
}

func decodeGB(raw []byte) (string, error) {
	out, err := decodeWith(raw, simplifiedchinese.GB18030.NewDecoder())
	if err != nil {
		return "", err
	}
	if strings.ContainsRune(out, '\uFFFD') {
		return "", fmt.Errorf("%w: invalid GBK/GB18030 bytes", ErrUndecodable)
	}
	return out, nil
}

func decodeWith(raw []byte, dec *encoding.Decoder) (string, error) {
	out, _, err := transform.Bytes(dec, raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUndecodable, err)
	}
	return string(out), nil
}

func declaredCharset(raw []byte) string {
	head := raw
	if len(head) > 4096 {
		head = head[:4096]
	}
	m := charsetRe.FindSubmatch(head)
	if m == nil {
		return ""
	}
	return strings.ToLower(string(m[1]))
}

func normalizeCharset(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "gb2312", "gb_2312-80", "gbk", "x-gbk", "cp936", "ms936":
		return "gbk"
	case "gb18030":
		return "gb18030"
	case "utf8", "utf-8":
		return "utf-8"
	default:
		return ""
	}
}
