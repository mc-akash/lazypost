package vars

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mc-akash/lazypost/internal/workspace"
)

type Source int

const (
	SrcGlobal Source = iota
	SrcShared
	SrcCollection
	SrcEnv
	SrcSecret
	SrcBuiltin
)

var sourceNames = [...]string{"global", "shared", "collection", "env", "secret", "builtin"}

func (s Source) String() string {
	if s < 0 || int(s) >= len(sourceNames) {
		return "unknown"
	}
	return sourceNames[s]
}

type Layer struct {
	Source Source
	Vars   map[string]string
}

type Resolver struct {
	layers []Layer
	now    func() time.Time
}

const maxDepth = 5

const randomIntMax = 1000

var RefPattern = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)

func NewResolver(layers ...Layer) *Resolver {
	return &Resolver{layers: append([]Layer(nil), layers...), now: time.Now}
}

func ForRequest(ws *workspace.Workspace, env *workspace.Env, req *workspace.Request) *Resolver {
	var layers []Layer
	if ws != nil {
		layers = append(layers, Layer{SrcGlobal, ws.Globals}, Layer{SrcShared, ws.SharedSecrets})
		if req != nil {
			for _, f := range ws.FolderChain(req) {
				layers = append(layers, Layer{SrcCollection, f.Vars})
			}
		}
	}
	if env != nil {
		layers = append(layers, Layer{SrcEnv, env.Vars}, Layer{SrcSecret, env.Secrets})
	}
	return NewResolver(layers...)
}

func (r *Resolver) Lookup(name string) (value string, src Source, ok bool) {
	if strings.HasPrefix(name, "$") {
		v, ok := r.builtin(name)
		return v, SrcBuiltin, ok
	}
	for i := len(r.layers) - 1; i >= 0; i-- {
		if v, ok := r.layers[i].Vars[name]; ok {
			return v, r.layers[i].Source, true
		}
	}
	return "", 0, false
}

func (r *Resolver) builtin(name string) (string, bool) {
	switch name {
	case "$timestamp":
		return strconv.FormatInt(r.now().Unix(), 10), true
	case "$isoTimestamp":
		return r.now().UTC().Format(time.RFC3339), true
	case "$guid", "$uuid":
		return uuidV4(), true
	case "$randomInt":
		n, err := rand.Int(rand.Reader, big.NewInt(randomIntMax+1))
		if err != nil {
			panic(fmt.Sprintf("vars: crypto/rand failed: %v", err))
		}
		return n.String(), true
	}
	return "", false
}

func uuidV4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("vars: crypto/rand failed: %v", err))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (r *Resolver) Expand(s string) (out string, missing []string) {
	e := expansion{r: r, seen: map[string]bool{}}
	out = e.expand(s, nil)
	return out, e.missing
}

type expansion struct {
	r       *Resolver
	missing []string
	seen    map[string]bool
}

func (e *expansion) expand(s string, stack []string) string {
	return RefPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := RefPattern.FindStringSubmatch(match)[1]
		v, src, ok := e.r.Lookup(name)
		if ok && src == SrcBuiltin {
			return v
		}
		if !ok || len(stack) >= maxDepth || contains(stack, name) {
			if !e.seen[name] {
				e.seen[name] = true
				e.missing = append(e.missing, name)
			}
			return match
		}
		return e.expand(v, append(stack[:len(stack):len(stack)], name))
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (r *Resolver) SecretValues() []string {
	set := map[string]bool{}
	for _, l := range r.layers {
		if l.Source != SrcShared && l.Source != SrcSecret {
			continue
		}
		for _, v := range l.Vars {
			if v != "" {
				set[v] = true
			}
		}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func RequestRefs(req workspace.Request) []string {
	var c refCollector
	c.add(req.URL)
	c.addKVs(req.Params)
	c.addKVs(req.Headers)
	a := req.Auth
	c.add(a.Token, a.Username, a.Password, a.Key, a.Value)
	c.add(req.Body.Content)
	c.addKVs(req.Body.Form)
	for _, p := range req.Body.Parts {
		c.add(p.Key, p.Value)
	}
	return c.names
}

type refCollector struct {
	names []string
	seen  map[string]bool
}

func (c *refCollector) add(ss ...string) {
	for _, s := range ss {
		for _, m := range RefPattern.FindAllStringSubmatch(s, -1) {
			if c.seen == nil {
				c.seen = map[string]bool{}
			}
			if !c.seen[m[1]] {
				c.seen[m[1]] = true
				c.names = append(c.names, m[1])
			}
		}
	}
}

func (c *refCollector) addKVs(kvs []workspace.KV) {
	for _, kv := range kvs {
		c.add(kv.Key, kv.Value)
	}
}
