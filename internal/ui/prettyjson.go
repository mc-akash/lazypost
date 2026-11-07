package ui

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

const templatePlaceholder = "lazypost-template-"

const jsonIndent = "  "

func prettyJSON(text string) (string, error) {
	masked, templates, prefix := maskTemplates(strings.TrimSpace(text))
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(masked), "", jsonIndent); err != nil {
		return "", err
	}
	out := buf.String()
	for i, tmpl := range templates {
		out = strings.Replace(out, placeholder(prefix, i), tmpl, 1)
	}
	return out, nil
}

func maskTemplates(s string) (masked string, templates []string, prefix string) {
	prefix = templatePlaceholder
	for strings.Contains(s, prefix) {
		prefix += "x"
	}
	var b strings.Builder
	inString := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case inString && c == '\\':
			end := min(i+2, len(s))
			b.WriteString(s[i:end])
			i = end
			continue
		case c == '"':
			inString = !inString
		case !inString:
			if n := templateLen(s[i:]); n > 0 {
				b.WriteString(placeholder(prefix, len(templates)))
				templates = append(templates, s[i:i+n])
				i += n
				continue
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String(), templates, prefix
}

func templateLen(s string) int {
	if !strings.HasPrefix(s, "{{") {
		return 0
	}
	end := strings.Index(s[2:], "}}")
	if end < 0 || strings.ContainsAny(s[2:2+end], "{}\"\n") {
		return 0
	}
	return 2 + end + 2
}

func placeholder(prefix string, i int) string {
	return `"` + prefix + strconv.Itoa(i) + `"`
}
