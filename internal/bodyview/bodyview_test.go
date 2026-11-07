package bodyview

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func wrap(tag string) func(string) string {
	return func(s string) string { return "<" + tag + ">" + s + "</" + tag + ">" }
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		ct   string
		body []byte
		want Kind
	}{
		{"json content type", "application/json; charset=utf-8", []byte(`{"a":1}`), KindJSON},
		{"vendor json content type", "application/vnd.api+json", []byte(`oops`), KindJSON},
		{"sniffed object", "text/plain", []byte("  {\"a\":1}\n"), KindJSON},
		{"sniffed array", "", []byte(`[1,2]`), KindJSON},
		{"brace but invalid", "", []byte(`{oops`), KindText},
		{"plain text", "text/plain", []byte("hello"), KindText},
		{"empty", "", nil, KindText},
		{"nul byte", "", []byte("ab\x00cd"), KindBinary},
		{"invalid utf8", "text/plain", []byte{0xff, 0xfe, 'a'}, KindBinary},
		{"binary beats json ct", "application/json", []byte{0x00}, KindBinary},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.ct, tc.body); got != tc.want {
				t.Fatalf("Detect = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectOnlyInspectsFirst8KiB(t *testing.T) {
	body := append(bytes.Repeat([]byte("a"), 8*1024), 0x00)
	if got := Detect("", body); got != KindText {
		t.Fatalf("NUL after 8 KiB: Detect = %v, want KindText", got)
	}
}

func TestDetectMultibyteRuneCutAt8KiB(t *testing.T) {

	body := append(bytes.Repeat([]byte("a"), 8*1024-1), []byte("é")...)
	if got := Detect("", body); got != KindText {
		t.Fatalf("split rune at window edge: Detect = %v, want KindText", got)
	}
}

func TestRenderPrettyJSON(t *testing.T) {
	got := Render([]byte(`{"id":42,"ok":true}`), "application/json", Palette{})
	want := []string{"{", `  "id": 42,`, `  "ok": true`, "}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderColorsKeysNotValues(t *testing.T) {
	p := Palette{Key: wrap("K"), String: wrap("S")}
	got := Render([]byte(`{"a":"b"}`), "application/json", p)
	if len(got) < 2 || got[1] != `  <K>"a"</K>: <S>"b"</S>` {
		t.Fatalf("got %q", got)
	}
}

func TestRenderColorsAllTokenKinds(t *testing.T) {
	p := Palette{Key: wrap("K"), String: wrap("S"), Number: wrap("N"), Bool: wrap("B"), Null: wrap("Z")}
	body := `{"s":"x\"y:","n":-1.5e3,"t":true,"f":false,"z":null,"arr":["q",2]}`
	got := Render([]byte(body), "", p)
	want := []string{
		"{",
		`  <K>"s"</K>: <S>"x\"y:"</S>,`,
		`  <K>"n"</K>: <N>-1.5e3</N>,`,
		`  <K>"t"</K>: <B>true</B>,`,
		`  <K>"f"</K>: <B>false</B>,`,
		`  <K>"z"</K>: <Z>null</Z>,`,
		`  <K>"arr"</K>: [`,
		`    <S>"q"</S>,`,
		`    <N>2</N>`,
		"  ]",
		"}",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRenderInvalidJSONFallsBackToRaw(t *testing.T) {
	got := Render([]byte(`{oops`), "application/json", Palette{})
	if !reflect.DeepEqual(got, []string{"{oops"}) {
		t.Fatalf("got %q", got)
	}
}

func TestRenderBinaryPlaceholder(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	got := Render(png, "image/png", Palette{})
	if !reflect.DeepEqual(got, []string{"binary body, 8 bytes"}) {
		t.Fatalf("got %q", got)
	}
}

func TestRenderBinaryPlaceholderCountsFullLength(t *testing.T) {
	body := append([]byte{0x00}, bytes.Repeat([]byte("a"), MaxRenderBytes+10)...)
	got := Render(body, "", Palette{})
	want := []string{fmt.Sprintf("binary body, %d bytes", len(body))}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderTruncatesLargeBody(t *testing.T) {
	line := strings.Repeat("x", 1023) + "\n"
	body := []byte(strings.Repeat(line, 3*1024))
	got := Render(body, "text/plain", Palette{})
	last := got[len(got)-1]
	if !strings.HasPrefix(last, "… truncated") {
		t.Fatalf("last line = %q", last)
	}
	if want := fmt.Sprintf("… truncated (%d bytes total)", len(body)); last != want {
		t.Fatalf("last line = %q, want %q", last, want)
	}
	if n := len(got) - 1; n != 2*1024 {
		t.Fatalf("rendered %d lines, want %d", n, 2*1024)
	}
}

func TestRenderTruncatedJSONRendersAsText(t *testing.T) {
	body := []byte(`{"a":"` + strings.Repeat("x", MaxRenderBytes) + `"}`)
	got := Render(body, "application/json", Palette{Key: wrap("K")})
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if !strings.HasPrefix(got[0], `{"a":"xxx`) || len(got[0]) != MaxRenderBytes {
		t.Fatalf("first line not raw truncated text (len %d)", len(got[0]))
	}
}

func TestRenderTextSplitsLinesAndDropsTrailingEmpty(t *testing.T) {
	got := Render([]byte("one\ntwo\n"), "text/plain", Palette{})
	if !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("got %q", got)
	}
}

func TestRenderEmptyBody(t *testing.T) {
	if got := Render(nil, "", Palette{}); len(got) != 0 {
		t.Fatalf("got %q, want no lines", got)
	}
}

func TestSearchIgnoresANSI(t *testing.T) {
	got := Search([]string{"nope", "\x1b[31mHello\x1b[0m", "say HELLO"}, "hello")
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchANSISplittingMatch(t *testing.T) {
	got := Search([]string{"He\x1b[1mllo"}, "hello")
	if !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("got %v", got)
	}
}

func TestSearchEmptyQueryMatchesNothing(t *testing.T) {
	if got := Search([]string{"a", "b"}, ""); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestRenderSanitizesControlCharacters(t *testing.T) {
	got := Render([]byte("a\x1b[2Jb\r\n\tc"), "text/plain", Palette{})
	want := []string{"a·[2Jb", "    c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderReplacesDELAndC1Controls(t *testing.T) {
	got := Render([]byte("x\x7fy\u009bz\x07"), "text/plain", Palette{})
	if !reflect.DeepEqual(got, []string{"x·y·z·"}) {
		t.Fatalf("got %q", got)
	}
}

func TestRenderJSONKeepsEscapedControlLiteral(t *testing.T) {
	p := Palette{String: wrap("S")}
	got := Render([]byte(`{"a":"x\u001b[2Jy"}`), "application/json", p)
	for _, l := range got {
		if strings.ContainsRune(l, 0x1b) {
			t.Fatalf("raw ESC in line %q", l)
		}
	}
	if got[1] != `  "a": <S>"x\u001b[2Jy"</S>` {
		t.Fatalf("got %q", got[1])
	}
}

func TestRenderSanitizesTruncatedBody(t *testing.T) {
	body := []byte("\x1b]52;c;evil\x07" + strings.Repeat("x", MaxRenderBytes))
	got := Render(body, "text/plain", Palette{})
	for _, l := range got {
		if strings.ContainsAny(l, "\x1b\x07") {
			t.Fatalf("control byte survived truncation path")
		}
	}
}

func TestRenderPaletteANSIUnaffected(t *testing.T) {
	esc := func(s string) string { return "\x1b[31m" + s + "\x1b[0m" }
	got := Render([]byte(`{"a":1}`), "application/json", Palette{Number: esc})
	if got[1] != "  \"a\": \x1b[31m1\x1b[0m" {
		t.Fatalf("got %q", got[1])
	}
}

func TestSanitize(t *testing.T) {
	tests := map[string]string{
		"plain":                  "plain",
		"a\r\nb":                 "a·b",
		"\tx":                    "    x",
		"200 OK\x1b]52;c;aGk=\a": "200 OK·]52;c;aGk=·",
		"a\x1bPq\x1b\\":          "a·Pq·\\",
		"del\x7f c1\u009b":       "del· c1·",
		"ünïcødé ok":             "ünïcødé ok",
	}
	for in, want := range tests {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
