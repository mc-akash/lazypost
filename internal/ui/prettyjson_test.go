package ui

import (
	"strings"
	"testing"
)

func TestPrettyJSONIndentsTwoSpaces(t *testing.T) {
	got, err := prettyJSON(`{"a":1,"b":[true,null],"c":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": [\n    true,\n    null\n  ],\n  \"c\": {}\n}"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrettyJSONIdempotent(t *testing.T) {
	for _, in := range []string{
		`{"a":1,"b":[1,2],"c":"x"}`,
		`{"id": {{user_id}}, "tags": [{{tag}}, "{{other}}"]}`,
		`[1, 2, 3]`,
	} {
		once, err := prettyJSON(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		twice, err := prettyJSON(once)
		if err != nil || twice != once {
			t.Fatalf("%q: second pass changed it (%v):\n%s\n---\n%s", in, err, once, twice)
		}
	}
}

func TestPrettyJSONQuotedTemplate(t *testing.T) {
	got, err := prettyJSON(`{"auth":"Bearer {{token}}","n":"{{ n }}"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"auth\": \"Bearer {{token}}\",\n  \"n\": \"{{ n }}\"\n}"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrettyJSONUnquotedTemplate(t *testing.T) {
	got, err := prettyJSON(`{"id": {{user_id}}, "list": [{{ a }},{{b}}], "s": "{{c}}"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"id\": {{user_id}},\n  \"list\": [\n    {{ a }},\n    {{b}}\n  ],\n  \"s\": \"{{c}}\"\n}"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrettyJSONPlaceholderClash(t *testing.T) {
	in := `{"k": "` + templatePlaceholder + `0", "id": {{id}}}`
	got, err := prettyJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"k\": \"" + templatePlaceholder + "0\",\n  \"id\": {{id}}\n}"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrettyJSONEscapedQuote(t *testing.T) {
	got, err := prettyJSON(`{"a":"x\" {{y}}"}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"a\": \"x\\\" {{y}}\"\n}"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPrettyJSONInvalid(t *testing.T) {
	for _, in := range []string{`{"a":`, `not json`, ``, `{"a":1} trailing`} {
		got, err := prettyJSON(in)
		if err == nil {
			t.Fatalf("%q: want an error, got %q", in, got)
		}
		if strings.Contains(err.Error(), templatePlaceholder) {
			t.Fatalf("%q: error leaks the placeholder: %v", in, err)
		}
	}
}

func TestPrettyJSONTrimsOuterSpace(t *testing.T) {
	got, err := prettyJSON("\n  {\"a\":1}  \n")
	if err != nil || got != "{\n  \"a\": 1\n}" {
		t.Fatalf("got %q, %v", got, err)
	}
}
