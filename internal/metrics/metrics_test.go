package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerExposesAllHeraldMetrics(t *testing.T) {
	m := New(Sources{
		RaftTerm:    func() float64 { return 3 },
		CommitIndex: func() float64 { return 42 },
		StaleAgents: func() float64 { return 1 },
	})
	m.RollbackTotal.Inc()
	m.PropagationLatencyMs.Observe(12)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"herald_propagation_latency_ms_bucket",
		"herald_raft_term 3",
		"herald_raft_commit_index 42",
		"herald_stale_agents 1",
		"herald_rollback_total 1",
		"herald_delivery_failures_total 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics output missing %q", want)
		}
	}
}

func TestNilSourcesReportZero(t *testing.T) {
	m := New(Sources{})

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if !strings.Contains(rec.Body.String(), "herald_raft_term 0") {
		t.Error("nil RaftTerm source should report 0")
	}
}