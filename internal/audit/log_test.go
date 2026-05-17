package audit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	log := NewLog(path)
	old := Event{At: time.Now().Add(-400 * 24 * time.Hour), Action: "public.start", Service: "127.0.0.1:3000"}
	recent := Event{At: time.Now(), Action: "public.stop", Service: "127.0.0.1:3000"}
	if err := log.Append(old); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(recent); err != nil {
		t.Fatal(err)
	}
	if err := log.Cleanup(365 * 24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	events, err := log.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != "public.stop" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
