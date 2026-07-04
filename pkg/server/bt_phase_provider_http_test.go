package server

import "testing"

func TestCloneMapNil(t *testing.T) {
	if got := cloneMap(nil); got != nil {
		t.Fatalf("expected nil clone, got %#v", got)
	}
}

func TestCloneMapCopy(t *testing.T) {
	src := map[string]any{"a": 1}
	dst := cloneMap(src)
	dst["a"] = 2
	if src["a"] != 1 {
		t.Fatalf("cloneMap mutated source: %#v", src)
	}
}

func TestStatusFromPhase(t *testing.T) {
	if got := statusFromPhase(""); got != "failure" {
		t.Fatalf("empty phase should map to failure, got %q", got)
	}
	if got := statusFromPhase("shape"); got != "success" {
		t.Fatalf("shape should map to success, got %q", got)
	}
}
