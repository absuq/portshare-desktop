package tailscale

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs map[string][]byte
	errs    map[string]error
}

func (r fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	if err := r.errs[key]; err != nil {
		return nil, err
	}
	output, ok := r.outputs[key]
	if !ok {
		return nil, fmt.Errorf("fake runner missing command: %s", key)
	}
	return output, nil
}

func TestCheckReadyReportsMissingCLI(t *testing.T) {
	client := NewClient(fakeRunner{errs: map[string]error{
		"tailscale status --json": errors.New("executable file not found"),
	}})
	report := client.CheckReady(context.Background())
	if report.Ready {
		t.Fatalf("expected not ready")
	}
	if report.Code != CodeTailscaleUnavailable {
		t.Fatalf("unexpected code: %s", report.Code)
	}
	if report.FixCommand != "" {
		t.Fatalf("missing CLI should not have automatic fix command")
	}
	if !strings.Contains(report.Message, "executable file not found") {
		t.Fatalf("expected message to include runner error, got %q", report.Message)
	}
}

func TestDiagnosticCodeStringsMatchContract(t *testing.T) {
	tests := map[DiagnosticCode]string{
		CodeTailscaleUnavailable: "tailscale.unavailable",
		CodeTailscaleStopped:     "tailscale.stopped",
		CodeNoTailscaleIP:        "tailscale.no_ip",
		CodePeerUnreachable:      "peer.unreachable",
		CodeDNSNotAccepted:       "dns.not_accepted",
	}
	for code, want := range tests {
		if string(code) != want {
			t.Fatalf("expected %s, got %s", want, code)
		}
	}
}

func TestPingPeerParsesDirectRoute(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale ping 100.109.251.97": []byte("pong from peer via 115.233.222.82:41641 in 15ms"),
	}})
	route, err := client.PingPeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}
	if route.Type != RouteDirect || route.Latency != "15ms" {
		t.Fatalf("unexpected route: %+v", route)
	}
}

func TestPingPeerPrefersDirectAfterDERP(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale ping 100.109.251.97": []byte(strings.Join([]string{
			"pong from peer via DERP(tok) in 88ms",
			"pong from peer via 115.233.222.82:41641 in 15ms",
		}, "\n")),
	}})

	route, err := client.PingPeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}
	if route.Type != RouteDirect || route.Via != "115.233.222.82:41641" || route.Latency != "15ms" {
		t.Fatalf("unexpected route: %+v", route)
	}
}

func TestPingPeerParsesPeerRelayRoute(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale ping 100.109.251.97": []byte("pong from peer via peer-relay(fra) in 42ms"),
	}})

	route, err := client.PingPeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}
	if route.Type != RoutePeerRelay || route.Via != "peer-relay(fra)" || route.Latency != "42ms" {
		t.Fatalf("unexpected route: %+v", route)
	}
}

func TestPingPeerParsesDERPRoute(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale ping 100.109.251.97": []byte("pong from peer via DERP(tok) in 88ms"),
	}})

	route, err := client.PingPeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}
	if route.Type != RouteDERP || route.Via != "DERP(tok)" || route.Latency != "88ms" {
		t.Fatalf("unexpected route: %+v", route)
	}
}

func TestCheckReadyReportsMagicDNSDisabled(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale status --json": []byte(`{
			"BackendState":"Running",
			"TailscaleIPs":["100.109.251.97"],
			"Self":{"HostName":"desk","DNSName":"desk.tailnet.ts.net."},
			"CurrentTailnet":{"MagicDNSEnabled":false,"MagicDNSSuffix":"tailnet.ts.net"}
		}`),
	}})

	report := client.CheckReady(context.Background())
	if report.Ready {
		t.Fatalf("expected not ready")
	}
	if report.Code != CodeDNSNotAccepted {
		t.Fatalf("unexpected code: %s", report.Code)
	}
	if report.FixCommand != "tailscale set --accept-dns=true" {
		t.Fatalf("unexpected fix command: %q", report.FixCommand)
	}
	if report.Status.LocalIPv4 != "100.109.251.97" {
		t.Fatalf("expected status to be preserved, got %+v", report.Status)
	}
}

func TestCheckReadyIncludesParseError(t *testing.T) {
	client := NewClient(fakeRunner{outputs: map[string][]byte{
		"tailscale status --json": []byte(`{`),
	}})

	report := client.CheckReady(context.Background())
	if report.Code != CodeTailscaleUnavailable {
		t.Fatalf("unexpected code: %s", report.Code)
	}
	if !strings.Contains(report.Message, "unexpected end of JSON input") {
		t.Fatalf("expected message to include parse error, got %q", report.Message)
	}
}
