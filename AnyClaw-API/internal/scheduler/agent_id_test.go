package scheduler

import "testing"

func TestNormalizeAgentID(t *testing.T) {
	if got := NormalizeAgentID(""); got != "main" {
		t.Errorf("empty = %q", got)
	}
	if got := NormalizeAgentID("Worker-B"); got != "worker-b" {
		t.Errorf("Worker-B = %q", got)
	}
}
