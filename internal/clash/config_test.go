package clash

import "testing"

func TestParseConfigYAMLReadsPortsControllerPipeAndTUN(t *testing.T) {
	raw := []byte(`
mixed-port: 7897
socks-port: 7898
port: 7899
external-controller: 127.0.0.1:9097
external-controller-pipe: \\.\pipe\verge-mihomo
secret: test-secret
tun:
  enable: true
`)

	cfg, err := ParseConfigYAML(raw)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.MixedPort != 7897 || cfg.SocksPort != 7898 || cfg.HTTPPort != 7899 {
		t.Fatalf("unexpected ports: %+v", cfg)
	}
	if cfg.ExternalController != "127.0.0.1:9097" {
		t.Fatalf("unexpected controller: %q", cfg.ExternalController)
	}
	if cfg.ExternalControllerPipe != `\\.\pipe\verge-mihomo` {
		t.Fatalf("unexpected pipe: %q", cfg.ExternalControllerPipe)
	}
	if cfg.Secret != "test-secret" {
		t.Fatalf("unexpected secret: %q", cfg.Secret)
	}
	if !cfg.TUNEnabled {
		t.Fatalf("expected TUN enabled: %+v", cfg)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := MaskSecret(""); got != "" {
		t.Fatalf("empty secret should remain empty, got %q", got)
	}
	if got := MaskSecret("abc"); got != "***" {
		t.Fatalf("non-empty secret should be masked, got %q", got)
	}
}
