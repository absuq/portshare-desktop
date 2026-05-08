package localhostbridge

import (
	"strings"
	"testing"
)

func TestParsePowerShellTCPListeners(t *testing.T) {
	input := `[
  {"LocalAddress":"127.0.0.1","LocalPort":18789},
  {"LocalAddress":"0.0.0.0","LocalPort":52726},
  {"LocalAddress":"::1","LocalPort":3000}
]`

	listeners, err := parsePowerShellTCPListeners([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(listeners) != 3 {
		t.Fatalf("unexpected listeners: %+v", listeners)
	}
	if listeners[0] != (ListeningPort{Address: "127.0.0.1", Port: 18789}) {
		t.Fatalf("unexpected first listener: %+v", listeners[0])
	}
}

func TestParsePowerShellTCPListenersAcceptsSingleObject(t *testing.T) {
	input := `{"LocalAddress":"127.0.0.1","LocalPort":18789}`

	listeners, err := parsePowerShellTCPListeners([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(listeners) != 1 || listeners[0].Port != 18789 {
		t.Fatalf("unexpected listeners: %+v", listeners)
	}
}

func TestParsePowerShellTCPListenersIgnoresEmptyOutput(t *testing.T) {
	listeners, err := parsePowerShellTCPListeners([]byte(strings.TrimSpace("")))
	if err != nil {
		t.Fatal(err)
	}
	if len(listeners) != 0 {
		t.Fatalf("unexpected listeners: %+v", listeners)
	}
}
