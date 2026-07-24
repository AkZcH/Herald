package targeting_test

import (
	"testing"

	"github.com/AkZcH/herald/internal/targeting"
)

func TestParse(t *testing.T) {
	cases := []struct {
		raw      string
		wantLen  int
		wantErr  bool
	}{
		{"env=prod,region=us-east", 2, false},
		{"env=prod", 1, false},
		{"", 0, false},
		{"env=", 1, false},
		{"=prod", 0, true},
		{"env", 0, true},
	}

	for _, c := range cases {
		sel, err := targeting.Parse(c.raw)
		if c.wantErr && err == nil {
			t.Errorf("Parse(%q): expected error", c.raw)
		}
		if !c.wantErr && err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", c.raw, err)
		}
		if !c.wantErr && len(sel) != c.wantLen {
			t.Errorf("Parse(%q): expected %d predicates, got %d", c.raw, c.wantLen, len(sel))
		}
	}
}

func TestMatches(t *testing.T) {
	sel, _ := targeting.Parse("env=prod,region=us-east")

	cases := []struct {
		labels map[string]string
		want   bool
	}{
		{map[string]string{"env": "prod", "region": "us-east"}, true},
		{map[string]string{"env": "prod", "region": "us-west"}, false},
		{map[string]string{"env": "staging", "region": "us-east"}, false},
		{map[string]string{"env": "prod"}, false},
		{map[string]string{}, false},
		{map[string]string{"env": "prod", "region": "us-east", "service": "payments"}, true},
	}

	for _, c := range cases {
		got := sel.Matches(c.labels)
		if got != c.want {
			t.Errorf("Matches(%v): expected %v, got %v", c.labels, c.want, got)
		}
	}
}

func TestEmptySelector(t *testing.T) {
	sel, _ := targeting.Parse("")
	if !sel.IsEmpty() {
		t.Error("expected empty selector")
	}
	if !sel.Matches(map[string]string{"env": "prod"}) {
		t.Error("empty selector should match everything")
	}
}