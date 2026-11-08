package path

import "testing"

func TestJoinAndOrigin(t *testing.T) {
	t.Parallel()
	origins := map[string]string{"": "defaults", "http": "file", "http.timeout": "env"}
	if got := Join("http", "timeout"); got != "http.timeout" {
		t.Fatalf("Join() = %q", got)
	}
	if got := Origin("http.timeout.unit", origins); got != "env" {
		t.Fatalf("Origin() = %q, want env", got)
	}
	if !Within("http.timeout", "http") || !Within("http.timeout.unit", "http.timeout") || Within("http", "http.timeout") {
		t.Fatal("Within() returned an unexpected result")
	}
}
