package clash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs  map[string][]byte
	commands []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	r.commands = append(r.commands, key)
	for match, output := range r.outputs {
		if strings.Contains(key, match) {
			return output, nil
		}
	}
	return []byte("[]"), nil
}

func TestDiscoverReadsConfigAndPrefersNamedPipeController(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "config.yaml"), `
mixed-port: 7897
socks-port: 7898
port: 7899
external-controller: 127.0.0.1:9097
secret: older
tun:
  enable: false
`)
	writeTestFile(t, filepath.Join(root, "clash-verge.yaml"), `
mixed-port: 7897
socks-port: 7898
port: 7899
external-controller: ""
external-controller-pipe: \\.\pipe\verge-mihomo
secret: active-secret
tun:
  enable: true
`)
	runner := &fakeRunner{outputs: map[string][]byte{
		"Get-NetAdapter": []byte(`[
			{"Name":"Meta","InterfaceDescription":"Meta Tunnel","ifIndex":19,"Status":"Up"},
			{"Name":"以太网","InterfaceDescription":"Realtek","ifIndex":15,"Status":"Up"}
		]`),
		"Get-NetTCPConnection": []byte(`[
			{"LocalAddress":"::","LocalPort":7897,"OwningProcess":20924},
			{"LocalAddress":"127.0.0.1","LocalPort":33331,"OwningProcess":23924}
		]`),
	}}
	service := NewService(runner, []string{root})

	report, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if report.Config.SourcePath != filepath.Join(root, "clash-verge.yaml") {
		t.Fatalf("expected active config to win, got %q", report.Config.SourcePath)
	}
	if report.Control.Kind != ControlNamedPipe || report.Control.Address != `\\.\pipe\verge-mihomo` {
		t.Fatalf("expected named pipe controller, got %+v", report.Control)
	}
	if report.Control.Secret != "active-secret" {
		t.Fatalf("expected active secret, got %q", report.Control.Secret)
	}
	if len(report.ProxyPorts) != 3 {
		t.Fatalf("expected 3 proxy entry ports, got %+v", report.ProxyPorts)
	}
	for _, port := range report.ProxyPorts {
		if port.Port == 7897 && port.Kind != "mixed" {
			t.Fatalf("7897 must be mixed proxy entry, got %+v", port)
		}
	}
	if len(report.TUNInterfaces) != 1 || report.TUNInterfaces[0].Name != "Meta" {
		t.Fatalf("expected Meta TUN interface, got %+v", report.TUNInterfaces)
	}
}

func TestDiscoverUsesHTTPControllerWhenPipeMissing(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "clash-verge.yaml"), `
external-controller: 127.0.0.1:9097
secret: active-secret
`)
	service := NewService(&fakeRunner{}, []string{root})

	report, err := service.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if report.Control.Kind != ControlHTTP || report.Control.Address != "http://127.0.0.1:9097" {
		t.Fatalf("expected HTTP controller, got %+v", report.Control)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
