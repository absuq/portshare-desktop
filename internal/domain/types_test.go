package domain

import "testing"

func TestShareModeStrings(t *testing.T) {
	if ModeTailnet.String() != "tailnet" {
		t.Fatalf("expected tailnet, got %q", ModeTailnet.String())
	}
	if ModePublic.String() != "public" {
		t.Fatalf("expected public, got %q", ModePublic.String())
	}
}
