package discovery

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeReadsTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><head><title>Vite App</title></head><body></body></html>"))
	}))
	defer server.Close()
	svc, err := Probe(server.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Title != "Vite App" {
		t.Fatalf("expected title, got %q", svc.Title)
	}
}
