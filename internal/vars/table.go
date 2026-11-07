package vars

import "strings"

type Row struct {
	Name, Value string
	Source      Source
	Missing     bool
	Secret      bool
}

func (r *Resolver) Table(refs []string) []Row {
	defined := map[string]bool{}
	for _, l := range r.layers {
		for name := range l.Vars {
			if !strings.HasPrefix(name, "$") {
				defined[name] = true
			}
		}
	}
	var rows []Row
	for _, name := range sortedKeys(defined) {
		v, src, _ := r.Lookup(name)
		rows = append(rows, Row{Name: name, Value: v, Source: src, Secret: isSecretSource(src)})
	}
	missing := map[string]bool{}
	for _, name := range refs {
		if !defined[name] && !strings.HasPrefix(name, "$") {
			missing[name] = true
		}
	}
	for _, name := range sortedKeys(missing) {
		rows = append(rows, Row{Name: name, Missing: true})
	}
	return rows
}

func isSecretSource(s Source) bool { return s == SrcShared || s == SrcSecret }

var secretHints = []string{"token", "secret", "password", "passwd", "apikey", "api_key", "key"}

func LooksSecret(name string) bool {
	lower := strings.ToLower(name)
	for _, h := range secretHints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

type WriteTarget int

const (
	ToEnv WriteTarget = iota
	ToEnvSecret
)

func PlanWrite(row Row) WriteTarget {
	if !row.Missing {
		switch row.Source {
		case SrcSecret, SrcShared:
			return ToEnvSecret
		case SrcEnv:
			return ToEnv
		}
	}
	if LooksSecret(row.Name) {
		return ToEnvSecret
	}
	return ToEnv
}
