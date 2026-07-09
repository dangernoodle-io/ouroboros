package dashboard

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// maxLineBytes bounds a single line's accumulation: readLine never buffers
// more than maxLineBytes for one line, regardless of producer input — any
// surplus is drained and discarded rather than appended, so memory stays
// bounded even for a pathological (e.g. multi-gigabyte) line. An oversized
// line is dropped as "line too long"; parsing continues (fail-open).
const maxLineBytes = 8 << 20 // 8MiB

// maxDropRawBytes bounds how much of an oversized line's raw text is kept
// in the Drop, so a huge line doesn't bloat the ParseResult.
const maxDropRawBytes = 200

// Drop records one NDJSON line the parser rejected, with the reason why.
type Drop struct {
	Line int
	Raw  string
	Err  string
}

// ParseResult holds every fragment the parser accepted plus every line it
// dropped, in reading order.
type ParseResult struct {
	Fragments []Fragment
	Dropped   []Drop
}

type peekLine struct {
	V    int    `json:"v"`
	Type string `json:"type"`
}

// Parse reads NDJSON fragments from r, fail-open: a malformed, invalid, or
// oversized line is recorded in Dropped and parsing continues. Parse
// returns an error only when the underlying reader itself fails (a genuine
// I/O error, not io.EOF).
func Parse(r io.Reader) (ParseResult, error) {
	result := ParseResult{Fragments: []Fragment{}, Dropped: []Drop{}}

	br := bufio.NewReader(r)

	lineNum := 0
	for {
		data, tooLong, err := readLine(br, maxLineBytes)
		if err != nil && !errors.Is(err, io.EOF) {
			return result, err
		}

		// readLine returns the trailing partial line together with io.EOF;
		// only stop once that final fragment is also empty.
		if len(data) == 0 && !tooLong && errors.Is(err, io.EOF) {
			break
		}
		lineNum++

		if tooLong {
			result.Dropped = append(result.Dropped, Drop{Line: lineNum, Raw: truncateRaw(string(data)), Err: "line too long"})
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}

		trimmed := strings.TrimRight(string(data), "\r")

		if strings.TrimSpace(trimmed) == "" {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}

		frag, dropReason := parseLine(trimmed)
		if dropReason != "" {
			result.Dropped = append(result.Dropped, Drop{Line: lineNum, Raw: trimmed, Err: dropReason})
		} else {
			result.Fragments = append(result.Fragments, frag)
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}

	return result, nil
}

// readLine reads one line (up to and including the terminating '\n', which
// is not returned) from r, accumulating at most capBytes bytes. If the line
// exceeds capBytes, tooLong is true and the surplus is drained to the next
// '\n' WITHOUT buffering it, so memory stays bounded by capBytes regardless
// of producer input. Returns io.EOF when the reader is exhausted (with any
// final partial line in data).
func readLine(r *bufio.Reader, capBytes int) (data []byte, tooLong bool, err error) {
	for {
		b, e := r.ReadByte()
		if e != nil {
			return data, tooLong, e
		}
		if b == '\n' {
			return data, tooLong, nil
		}
		if len(data) < capBytes {
			data = append(data, b)
		} else {
			tooLong = true
		}
	}
}

// truncateRaw shortens an oversized line's raw text for storage in a Drop,
// so a pathological line doesn't bloat the ParseResult, backing off to a
// valid UTF-8 boundary so the result is never a mid-rune-truncated string.
func truncateRaw(s string) string {
	if len(s) <= maxDropRawBytes {
		return s
	}
	t := s[:maxDropRawBytes]
	for len(t) > 0 && !utf8.ValidString(t) {
		t = t[:len(t)-1]
	}
	return t + "…"
}

// parseLine unmarshals and validates a single NDJSON line. On success it
// returns the concrete Fragment and an empty drop reason; on failure it
// returns a nil Fragment and a non-empty reason.
func parseLine(line string) (Fragment, string) {
	var peek peekLine
	if err := json.Unmarshal([]byte(line), &peek); err != nil {
		return nil, "malformed json: " + err.Error()
	}
	if peek.V != schemaVersion {
		return nil, "unsupported version"
	}

	switch peek.Type {
	case "tile":
		var t Tile
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return nil, "malformed json: " + err.Error()
		}
		if t.Label == "" || t.Value == "" {
			return nil, "tile: label and value required"
		}
		return t, ""
	case "bar":
		var b Bar
		if err := json.Unmarshal([]byte(line), &b); err != nil {
			return nil, "malformed json: " + err.Error()
		}
		if b.Label == "" {
			return nil, "bar: label required"
		}
		if b.Value < 0 {
			return nil, "bar: value must be >= 0"
		}
		if b.Max != nil && *b.Max <= 0 {
			return nil, "bar: max must be > 0"
		}
		return b, ""
	case "group":
		var g Group
		if err := json.Unmarshal([]byte(line), &g); err != nil {
			return nil, "malformed json: " + err.Error()
		}
		if len(g.Cards) < 1 {
			return nil, "group: at least one card required"
		}
		for _, c := range g.Cards {
			if c.Title == "" {
				return nil, "group: every card requires a title"
			}
		}
		return g, ""
	case "note":
		var n Note
		if err := json.Unmarshal([]byte(line), &n); err != nil {
			return nil, "malformed json: " + err.Error()
		}
		if n.Text == "" {
			return nil, "note: text required"
		}
		return n, ""
	case "html":
		var h HTML
		if err := json.Unmarshal([]byte(line), &h); err != nil {
			return nil, "malformed json: " + err.Error()
		}
		if h.HTML == "" {
			return nil, "html: html required"
		}
		return h, ""
	default:
		return nil, "unknown fragment type"
	}
}
