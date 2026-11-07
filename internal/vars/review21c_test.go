package vars

import (
	"reflect"
	"strconv"
	"testing"
)

func TestExpandReportsCycleAndDepthAsMissing(t *testing.T) {
	cycle := NewResolver(Layer{SrcEnv, map[string]string{"a": "{{b}}", "b": "{{a}}", "ok": "fine"}})
	if out, missing := cycle.Expand("{{ok}} {{a}} {{b}}"); out != "fine {{a}} {{b}}" || !reflect.DeepEqual(missing, []string{"a", "b"}) {
		t.Errorf("cycle: out %q missing %v", out, missing)
	}

	deep := map[string]string{"v9": "end"}
	for i := range 9 {
		deep["v"+strconv.Itoa(i)] = "{{v" + strconv.Itoa(i+1) + "}}"
	}
	if _, missing := NewResolver(Layer{SrcEnv, deep}).Expand("{{v0}}"); len(missing) != 1 {
		t.Errorf("too deep: missing %v, want one entry", missing)
	}
}
