package tailscale

import "testing"

func TestParseStatusReturnsLocalIPv4AndDNS(t *testing.T) {
	raw := []byte(`{
		"BackendState":"Running",
		"TailscaleIPs":["100.79.83.104","fd7a:115c:a1e0::1"],
		"Self":{"HostName":"abs_u_q","DNSName":"abs-u-q.tail51fe78.ts.net."},
		"CurrentTailnet":{"MagicDNSEnabled":true,"MagicDNSSuffix":"tail51fe78.ts.net"}
	}`)
	status, err := ParseStatus(raw)
	if err != nil {
		t.Fatal(err)
	}
	if status.BackendState != "Running" {
		t.Fatalf("unexpected backend state: %q", status.BackendState)
	}
	if status.LocalIPv4 != "100.79.83.104" {
		t.Fatalf("expected IPv4, got %q", status.LocalIPv4)
	}
	if status.SelfDNSName != "abs-u-q.tail51fe78.ts.net" {
		t.Fatalf("expected trimmed DNS name, got %q", status.SelfDNSName)
	}
	if !status.MagicDNSEnabled {
		t.Fatalf("expected MagicDNS enabled")
	}
}
