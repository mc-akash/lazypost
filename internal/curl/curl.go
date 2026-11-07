package curl

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/google/shlex"

	"github.com/mc-akash/lazypost/internal/workspace"
)

func Export(req workspace.Request) string {
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}
	bodyArgs := exportBody(req.Body)

	args := []string{"curl"}
	if method != "GET" || len(bodyArgs) > 0 {
		args = append(args, "-X", quote(method))
	}
	args = append(args, quote(withParams(req.URL, req.Params)))
	for _, h := range req.Headers {
		if !h.Disabled {
			args = append(args, "-H", quote(h.Key+": "+h.Value))
		}
	}
	args = append(args, bodyArgs...)
	return strings.Join(args, " ")
}

func exportBody(b workspace.Body) []string {
	var args []string
	switch b.Type {
	case "json", "raw":
		if b.Content != "" {
			args = append(args, "--data-raw", quote(b.Content))
		}
	case "form":
		for _, kv := range b.Form {
			if !kv.Disabled {
				args = append(args, "--data-urlencode", quote(kv.Key+"="+kv.Value))
			}
		}
	case "multipart":
		for _, p := range b.Parts {
			if p.Disabled {
				continue
			}
			if p.File {
				args = append(args, "-F", quote(p.Key+"=@"+p.Value))
			} else {
				args = append(args, "--form-string", quote(p.Key+"="+p.Value))
			}
		}
	}
	return args
}

func withParams(rawURL string, params []workspace.KV) string {
	var pairs []string
	for _, p := range params {
		if !p.Disabled {
			pairs = append(pairs, url.QueryEscape(p.Key)+"="+url.QueryEscape(p.Value))
		}
	}
	if len(pairs) == 0 {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + strings.Join(pairs, "&")
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var ignoredFlags = map[string]bool{
	"-s": true, "-S": true, "-L": true, "-k": true, "-i": true, "-v": true,
	"--compressed": true, "--location": true, "--silent": true, "--insecure": true,
	"--http1.1": true, "--http2": true, "--http2-prior-knowledge": true,
	"--globoff": true, "-g": true, "--fail": true, "-f": true,
}

type dataKind int

const (
	dataNone dataKind = iota
	dataRaw
	dataURLEncode
	dataForm
)

type importState struct {
	req      workspace.Request
	rawURL   string
	kind     dataKind
	rawParts []string
}

func JoinLines(cmd string) string {
	return strings.NewReplacer("\\\r\n", " ", "\\\n", " ").Replace(cmd)
}

func Import(cmd string) (workspace.Request, error) {
	cmd, err := expandANSIC(JoinLines(cmd))
	if err != nil {
		return workspace.Request{}, err
	}
	tokens, err := shlex.Split(cmd)
	if err != nil {
		return workspace.Request{}, fmt.Errorf("parse curl command: %w", err)
	}
	if len(tokens) > 0 && tokens[0] == "curl" {
		tokens = tokens[1:]
	}

	st := &importState{}
	for i := 0; i < len(tokens); i++ {
		flag, value, hasValue := splitFlag(tokens[i])
		if flag == "" {
			if err := st.setURL(tokens[i]); err != nil {
				return workspace.Request{}, err
			}
			continue
		}
		if ignoredFlags[flag] {
			continue
		}
		if !hasValue {
			i++
			if i >= len(tokens) {
				return workspace.Request{}, fmt.Errorf("flag %s requires a value", flag)
			}
			value = tokens[i]
		}
		if err := st.apply(flag, value); err != nil {
			return workspace.Request{}, err
		}
	}
	return st.finish()
}

var valueFlags = map[string]string{
	"-X":               "-X",
	"--request":        "-X",
	"-H":               "-H",
	"--header":         "-H",
	"-d":               "-d",
	"--data":           "-d",
	"--data-raw":       "-d",
	"--data-binary":    "-d",
	"--data-ascii":     "-d",
	"--data-urlencode": "--data-urlencode",
	"-F":               "-F",
	"--form":           "-F",
	"--form-string":    "--form-string",
	"-u":               "-u",
	"--user":           "-u",
	"--url":            "--url",
	"-b":               "-b",
	"--cookie":         "-b",
	"-A":               "-A",
	"--user-agent":     "-A",
	"-e":               "-e",
	"--referer":        "-e",
}

func splitFlag(tok string) (flag, value string, hasValue bool) {
	if !strings.HasPrefix(tok, "-") || tok == "-" {
		return "", "", false
	}
	if strings.HasPrefix(tok, "--") {
		if k, v, ok := strings.Cut(tok, "="); ok {
			return k, v, true
		}
		return tok, "", false
	}
	for len(tok) > 2 && ignoredFlags[tok[:2]] {
		tok = "-" + tok[2:]
	}
	if _, ok := valueFlags[tok[:2]]; ok && len(tok) > 2 {
		return tok[:2], tok[2:], true
	}
	return tok, "", false
}

func (st *importState) setURL(u string) error {
	if st.rawURL != "" {
		return fmt.Errorf("more than one URL given: %q and %q", st.rawURL, u)
	}
	st.rawURL = u
	return nil
}

func (st *importState) apply(flag, value string) error {
	canon, ok := valueFlags[flag]
	if !ok {
		return fmt.Errorf("unsupported curl flag %s", flag)
	}
	switch canon {
	case "-X":
		if !workspace.ValidMethod(value) {
			return fmt.Errorf("invalid method %q", value)
		}
		st.req.Method = strings.ToUpper(value)
	case "-H":
		k, v, ok := strings.Cut(value, ":")
		if !ok {
			return fmt.Errorf("invalid header %q: missing ':'", value)
		}
		st.addHeader(strings.TrimSpace(k), strings.TrimSpace(v))
	case "-d":
		if flag != "--data-raw" && strings.HasPrefix(value, "@") {
			return fmt.Errorf("%s %s: reading data from a file is not supported", flag, value)
		}
		if err := st.setKind(dataRaw); err != nil {
			return err
		}
		st.rawParts = append(st.rawParts, value)
	case "--data-urlencode":
		if err := st.setKind(dataURLEncode); err != nil {
			return err
		}
		k, v, _ := strings.Cut(value, "=")
		st.req.Body.Form = append(st.req.Body.Form, workspace.KV{Key: k, Value: v})
	case "-F", "--form-string":
		if err := st.setKind(dataForm); err != nil {
			return err
		}
		k, v, _ := strings.Cut(value, "=")
		part := workspace.Part{Key: k, Value: v}
		if canon == "-F" && strings.HasPrefix(v, "@") {
			part.Value, part.File = v[1:], true
		}
		st.req.Body.Parts = append(st.req.Body.Parts, part)
	case "-u":
		user, pass, _ := strings.Cut(value, ":")
		st.req.Auth = workspace.Auth{Type: "basic", Username: user, Password: pass}
	case "--url":
		return st.setURL(value)
	case "-b":
		if !strings.Contains(value, "=") {
			return fmt.Errorf("%s %q: cookie-jar files are not supported", flag, value)
		}
		st.addHeader("Cookie", value)
	case "-A":
		st.addHeader("User-Agent", value)
	case "-e":
		st.addHeader("Referer", value)
	}
	return nil
}

func (st *importState) addHeader(key, value string) {
	st.req.Headers = append(st.req.Headers, workspace.KV{Key: key, Value: value})
}

func (st *importState) setKind(k dataKind) error {
	if st.kind != dataNone && st.kind != k {
		return errors.New("cannot combine -d/--data, --data-urlencode and -F/--form in one request")
	}
	st.kind = k
	return nil
}

func (st *importState) finish() (workspace.Request, error) {
	if st.rawURL == "" {
		return workspace.Request{}, errors.New("no URL in curl command")
	}
	req := st.req
	req.URL, req.Params = splitQuery(st.rawURL)

	switch st.kind {
	case dataNone:
		req.Body.Type = "none"
	case dataRaw:
		req.Body.Content = strings.Join(st.rawParts, "&")
		req.Body.Type = "raw"
		if isJSON(req.Headers, req.Body.Content) {
			req.Body.Type = "json"
		} else if !slices.ContainsFunc(req.Headers, func(h workspace.KV) bool { return strings.EqualFold(h.Key, "Content-Type") }) {
			req.Headers = append(req.Headers, workspace.KV{Key: "Content-Type", Value: "application/x-www-form-urlencoded"})
		}
	case dataURLEncode:
		req.Body.Type = "form"
	case dataForm:
		req.Body.Type = "multipart"
	}
	if req.Method == "" {
		req.Method = "GET"
		if st.kind != dataNone {
			req.Method = "POST"
		}
	}
	req.Name = req.Method + " " + urlPath(req.URL)
	return req, nil
}

func isJSON(headers []workspace.KV, content string) bool {
	for _, h := range headers {
		if strings.EqualFold(h.Key, "Content-Type") && strings.Contains(strings.ToLower(h.Value), "json") {
			return true
		}
	}
	return strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[")
}

func splitQuery(rawURL string) (string, []workspace.KV) {
	base, _, _ := strings.Cut(rawURL, "#")
	base, query, ok := strings.Cut(base, "?")
	if !ok || query == "" {
		return base, nil
	}
	var params []workspace.KV
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		params = append(params, workspace.KV{Key: unescape(k), Value: unescape(v)})
	}
	return base, params
}

func unescape(s string) string {
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}

func urlPath(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		return u.Path
	}
	return "/"
}

func expandANSIC(cmd string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\\' && i+1 < len(cmd):
			out.WriteString(cmd[i : i+2])
			i++
		case c == '\'' || c == '"':
			end := closingQuote(cmd, i)
			out.WriteString(cmd[i:end])
			i = end - 1
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'':
			decoded, end, err := decodeANSIC(cmd, i+2)
			if err != nil {
				return "", err
			}
			out.WriteString(quote(decoded))
			i = end
		default:
			out.WriteByte(c)
		}
	}
	return out.String(), nil
}

func closingQuote(cmd string, start int) int {
	q := cmd[start]
	for i := start + 1; i < len(cmd); i++ {
		if q == '"' && cmd[i] == '\\' {
			i++
			continue
		}
		if cmd[i] == q {
			return i + 1
		}
	}
	return len(cmd)
}

func decodeANSIC(cmd string, start int) (string, int, error) {
	var out strings.Builder
	for i := start; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' {
			return out.String(), i, nil
		}
		if c != '\\' || i+1 >= len(cmd) {
			out.WriteByte(c)
			continue
		}
		i++
		switch e := cmd[i]; e {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case '\\', '\'', '"':
			out.WriteByte(e)
		case 'x', 'u':
			maxDigits := 2
			if e == 'u' {
				maxDigits = 4
			}
			r, n := parseHex(cmd[i+1:], maxDigits)
			if n == 0 {
				out.WriteByte('\\')
				out.WriteByte(e)
				continue
			}
			if e == 'x' {
				out.WriteByte(byte(r))
			} else {
				out.WriteRune(rune(r))
			}
			i += n
		default:
			out.WriteByte('\\')
			out.WriteByte(e)
		}
	}
	return "", 0, fmt.Errorf("unterminated $' quote in curl command")
}

func parseHex(s string, maxDigits int) (int, int) {
	v, n := 0, 0
	for n < maxDigits && n < len(s) {
		d := strings.IndexByte("0123456789abcdef", toLower(s[n]))
		if d < 0 {
			break
		}
		v = v*16 + d
		n++
	}
	return v, n
}

func toLower(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}
