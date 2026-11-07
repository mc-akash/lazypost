package bodyview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Kind int

const (
	KindText Kind = iota
	KindJSON
	KindBinary
)

const MaxRenderBytes = 2 << 20

const sniffBytes = 8 * 1024

type Palette struct {
	Key, String, Number, Bool, Null func(string) string
}

func Detect(contentType string, body []byte) Kind {
	if isBinary(body) {
		return KindBinary
	}
	if strings.Contains(strings.ToLower(contentType), "json") {
		return KindJSON
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid(trimmed) {
		return KindJSON
	}
	return KindText
}

func isBinary(body []byte) bool {
	head := body
	if len(head) > sniffBytes {
		head = trimPartialRune(head[:sniffBytes])
	}
	return bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(head)
}

func trimPartialRune(b []byte) []byte {
	for i := 1; i < utf8.UTFMax && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < utf8.RuneSelf {
			return b
		}
		if utf8.RuneStart(c) {
			if !utf8.FullRune(b[len(b)-i:]) {
				return b[:len(b)-i]
			}
			return b
		}
	}
	return b
}

func Render(body []byte, contentType string, p Palette) []string {
	kind := Detect(contentType, body)
	if kind == KindBinary {
		return []string{fmt.Sprintf("binary body, %d bytes", len(body))}
	}
	if len(body) > MaxRenderBytes {
		lines := textLines(body[:MaxRenderBytes])
		return append(lines, fmt.Sprintf("… truncated (%d bytes total)", len(body)))
	}
	if kind == KindJSON {
		var buf bytes.Buffer
		if err := json.Indent(&buf, bytes.TrimSpace(body), "", "  "); err == nil {
			return colorLines(textLines(buf.Bytes()), p)
		}
	}
	return textLines(body)
}

func textLines(b []byte) []string {
	if len(b) == 0 {
		return []string{}
	}
	lines := strings.Split(string(b), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = Sanitize(l)
	}
	return lines
}

const tabWidth = 4

const controlPlaceholder = '·'

func Sanitize(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == '\r':
		case r == '\t':
			sb.WriteString(strings.Repeat(" ", tabWidth))
		case isControl(r):
			sb.WriteRune(controlPlaceholder)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

func colorLines(lines []string, p Palette) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = colorLine(l, p)
	}
	return out
}

func colorLine(line string, p Palette) string {
	var sb strings.Builder
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == '"':
			end := stringEnd(line, i)
			tok := line[i:end]
			if end < len(line) && line[end] == ':' {
				sb.WriteString(apply(p.Key, tok))
			} else {
				sb.WriteString(apply(p.String, tok))
			}
			i = end
		case c == '-' || (c >= '0' && c <= '9'):
			end := i + 1
			for end < len(line) && strings.IndexByte("0123456789.eE+-", line[end]) >= 0 {
				end++
			}
			sb.WriteString(apply(p.Number, line[i:end]))
			i = end
		case strings.HasPrefix(line[i:], "true"):
			sb.WriteString(apply(p.Bool, "true"))
			i += 4
		case strings.HasPrefix(line[i:], "false"):
			sb.WriteString(apply(p.Bool, "false"))
			i += 5
		case strings.HasPrefix(line[i:], "null"):
			sb.WriteString(apply(p.Null, "null"))
			i += 4
		default:
			sb.WriteByte(c)
			i++
		}
	}
	return sb.String()
}

func stringEnd(line string, start int) int {
	for i := start + 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(line)
}

func apply(f func(string) string, s string) string {
	if f == nil {
		return s
	}
	return f(s)
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func Search(lines []string, query string) []int {
	if query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var hits []int
	for i, l := range lines {
		if strings.Contains(strings.ToLower(ansiRe.ReplaceAllString(l, "")), q) {
			hits = append(hits, i)
		}
	}
	return hits
}
