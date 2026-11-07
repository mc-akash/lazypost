package vars

import (
	"reflect"
	"testing"
)

func TestTable(t *testing.T) {
	r := NewResolver(
		Layer{SrcGlobal, map[string]string{"zeta": "z", "base_url": "g"}},
		Layer{SrcShared, map[string]string{"api_key": "k"}},
		Layer{SrcEnv, map[string]string{"base_url": "http://x"}},
		Layer{SrcSecret, map[string]string{"token": "t"}})
	got := r.Table([]string{"trace_id", "base_url", "$uuid", "alpha", "trace_id"})
	want := []Row{
		{Name: "api_key", Value: "k", Source: SrcShared, Secret: true},
		{Name: "base_url", Value: "http://x", Source: SrcEnv},
		{Name: "token", Value: "t", Source: SrcSecret, Secret: true},
		{Name: "zeta", Value: "z", Source: SrcGlobal},
		{Name: "alpha", Missing: true},
		{Name: "trace_id", Missing: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Table =\n%+v\nwant\n%+v", got, want)
	}
	if last := got[len(got)-1]; last.Name != "trace_id" || !last.Missing {
		t.Errorf("last row = %+v", last)
	}
}

func TestLooksSecret(t *testing.T) {
	for _, n := range []string{"TOKEN", "client_secret", "Password", "passwd", "ApiKey", "API_KEY", "ssh_key"} {
		if !LooksSecret(n) {
			t.Errorf("LooksSecret(%q) = false", n)
		}
	}
	for _, n := range []string{"base_url", "user_id", "api_ver"} {
		if LooksSecret(n) {
			t.Errorf("LooksSecret(%q) = true", n)
		}
	}
}

func TestPlanWrite(t *testing.T) {
	cases := []struct {
		row  Row
		want WriteTarget
	}{
		{Row{Name: "base_url", Source: SrcEnv}, ToEnv},
		{Row{Name: "x", Source: SrcSecret}, ToEnvSecret},
		{Row{Name: "x", Source: SrcShared}, ToEnvSecret},
		{Row{Name: "api_ver", Source: SrcGlobal}, ToEnv},
		{Row{Name: "api_key", Source: SrcCollection}, ToEnvSecret},
		{Row{Name: "token", Missing: true}, ToEnvSecret},
		{Row{Name: "user_id", Missing: true}, ToEnv},
		{Row{Name: "password", Source: SrcEnv}, ToEnv},
	}
	for _, c := range cases {
		if got := PlanWrite(c.row); got != c.want {
			t.Errorf("PlanWrite(%+v) = %v, want %v", c.row, got, c.want)
		}
	}
}
