package vectorclock_test

import (
	"testing"

	"github.com/AkZcH/herald/internal/vectorclock"
)

func TestIncrement(t *testing.T) {
	vc := vectorclock.New()
	vc = vc.Increment("node-1")
	vc = vc.Increment("node-1")
	vc = vc.Increment("node-2")

	if vc["node-1"] != 2 {
		t.Errorf("expected node-1=2, got %d", vc["node-1"])
	}
	if vc["node-2"] != 1 {
		t.Errorf("expected node-2=1, got %d", vc["node-2"])
	}
}

func TestHappensBefore(t *testing.T) {
	a := vectorclock.New().Increment("node-1")
	b := a.Increment("node-1")

	if !a.HappensBefore(b) {
		t.Error("expected a to happen before b")
	}
	if b.HappensBefore(a) {
		t.Error("expected b not to happen before a")
	}
}

func TestConcurrent(t *testing.T) {
	a := vectorclock.New().Increment("node-1")
	b := vectorclock.New().Increment("node-2")

	if !a.Concurrent(b) {
		t.Error("expected a and b to be concurrent")
	}
}

func TestMerge(t *testing.T) {
	a := vectorclock.New().Increment("node-1").Increment("node-1")
	b := vectorclock.New().Increment("node-2")

	merged := a.Merge(b)

	if merged["node-1"] != 2 {
		t.Errorf("expected node-1=2, got %d", merged["node-1"])
	}
	if merged["node-2"] != 1 {
		t.Errorf("expected node-2=1, got %d", merged["node-2"])
	}
}

func TestJSONRoundtrip(t *testing.T) {
	vc := vectorclock.New().Increment("node-1").Increment("node-2")

	data, err := vc.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}

	restored, err := vectorclock.FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}

	if restored["node-1"] != vc["node-1"] {
		t.Errorf("node-1 mismatch after roundtrip")
	}
	if restored["node-2"] != vc["node-2"] {
		t.Errorf("node-2 mismatch after roundtrip")
	}
}