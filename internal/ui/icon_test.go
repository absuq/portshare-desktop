package ui

import (
	"bytes"
	"image/png"
	"testing"
)

func TestPortshareIconResourceIsPNG(t *testing.T) {
	icon := portshareIconResource()
	if icon == nil {
		t.Fatal("expected icon resource")
	}
	if icon.Name() != "portshare.png" {
		t.Fatalf("expected PNG resource name, got %q", icon.Name())
	}
	if _, err := png.Decode(bytes.NewReader(icon.Content())); err != nil {
		t.Fatalf("expected decodable PNG icon: %v", err)
	}
}
