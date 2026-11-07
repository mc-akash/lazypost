package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mc-akash/lazypost/internal/vars"
	"github.com/mc-akash/lazypost/internal/workspace"
)

type importSecret struct{ name, value string }

type secretNamer struct {
	env   *workspace.Env
	taken map[string]bool
}

func (n *secretNamer) name(base, value string) string {
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s_%d", base, i)
		}
		if n.taken[name] {
			continue
		}
		_, inVars := n.env.Vars[name]
		old, inSecrets := n.env.Secrets[name]
		if !inVars && (!inSecrets || old == value) {
			n.taken[name] = true
			return name
		}
	}
}

func extractSecrets(imported workspace.Request, name string, env *workspace.Env) (workspace.Request, []importSecret) {
	slug := strings.ReplaceAll(workspace.Slug(name), "-", "_")
	namer := &secretNamer{env: env, taken: map[string]bool{}}
	var secrets []importSecret
	ref := func(suffix, value string) string {
		v := namer.name(slug+"_"+suffix, value)
		secrets = append(secrets, importSecret{name: v, value: value})
		return "{{" + v + "}}"
	}

	out := imported
	out.Headers = slices.Clone(imported.Headers)
	for i, h := range out.Headers {
		switch {
		case strings.EqualFold(h.Key, "Authorization"), strings.EqualFold(h.Key, "Proxy-Authorization"):
			scheme, cred, hasScheme := strings.Cut(strings.TrimSpace(h.Value), " ")
			cred = strings.TrimSpace(cred)
			if !hasScheme {
				scheme, cred = "", scheme
			}
			if cred == "" || onlyRefs(cred) {
				continue
			}
			if scheme != "" {
				scheme += " "
			}
			out.Headers[i].Value = scheme + ref("authorization", cred)
		case strings.EqualFold(h.Key, "Cookie") && literalCookie(h.Value):
			out.Headers[i].Value = ref("cookie", strings.TrimSpace(h.Value))
		}
	}

	for _, f := range []struct {
		suffix string
		value  *string
	}{
		{"password", &out.Auth.Password},
		{"token", &out.Auth.Token},
		{"apikey", &out.Auth.Value},
	} {
		if v := *f.value; strings.TrimSpace(v) != "" && !onlyRefs(v) {
			*f.value = ref(f.suffix, v)
		}
	}
	return out, secrets
}

func onlyRefs(v string) bool {
	return vars.RefPattern.MatchString(v) && strings.TrimSpace(vars.RefPattern.ReplaceAllString(v, "")) == ""
}

func literalCookie(v string) bool {
	for _, c := range strings.Split(v, ";") {
		_, val, found := strings.Cut(c, "=")
		if !found {
			val = c
		}
		if strings.TrimSpace(val) != "" && !onlyRefs(val) {
			return true
		}
	}
	return false
}

var errSaveNeedsEnv = errors.New("save needs an environment")

func statusSaveNeedsEnv() string {
	return "credentials need an environment: activate one (" + km.SwitchEnv[0] + ") before saving"
}

func (m Model) routeSaveSecrets(req workspace.Request, name string) (workspace.Request, string, error) {
	if !hasImportSecrets(req, name) {
		return req, "", nil
	}
	env := m.activeEnv()
	if env == nil {
		return req, "", errSaveNeedsEnv
	}
	return m.moveImportSecrets(req, name, env)
}

func saveFailStatus(prefix string, err error) string {
	if errors.Is(err, errSaveNeedsEnv) {
		return statusSaveNeedsEnv()
	}
	return prefix + err.Error()
}

func secretsNote(n int, env string) string {
	noun := "secrets"
	if n == 1 {
		noun = "secret"
	}
	return fmt.Sprintf(" (%d %s → %s)", n, noun, env)
}
