package netdiag

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type recordingRunner struct {
	outputs  map[string][]byte
	commands []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	r.commands = append(r.commands, key)
	for match, output := range r.outputs {
		if strings.Contains(key, match) {
			return output, nil
		}
	}
	return nil, fmt.Errorf("missing fake output for %s", key)
}

func TestDiagnosePeerReportsDirectProxyRoute(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"tailscale ping --c 10 100.109.251.97": []byte("pong from desktop via 115.233.222.82:41641 in 249ms"),
		"Find-NetRoute": []byte(`{
			"InterfaceAlias":"Meta",
			"InterfaceIndex":19,
			"NextHop":"198.18.0.2",
			"RouteMetric":0,
			"InterfaceMetric":0,
			"IPAddress":"198.18.0.1"
		}`),
		"Get-NetRoute -DestinationPrefix": []byte(`[
			{"InterfaceAlias":"Meta","InterfaceIndex":19,"NextHop":"198.18.0.2","RouteMetric":0,"InterfaceMetric":0,"InterfaceIP":"198.18.0.1"},
			{"InterfaceAlias":"以太网","InterfaceIndex":15,"NextHop":"192.168.1.1","RouteMetric":0,"InterfaceMetric":25,"InterfaceIP":"192.168.1.11"}
		]`),
		"tailscale netcheck --format json --bind-address 198.18.0.1":   []byte(`{"UDP":true,"GlobalV4":"172.105.240.197:43051","GlobalV6":""}`),
		"tailscale netcheck --format json --bind-address 192.168.1.11": []byte(`{"UDP":true,"GlobalV4":"112.10.189.69:1142","GlobalV6":""}`),
	}}
	service := NewService(runner)

	report, err := service.DiagnosePeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != PathDirectProxy {
		t.Fatalf("expected proxy path, got %+v", report)
	}
	if report.EndpointIP != "115.233.222.82" || report.CurrentRoute.InterfaceAlias != "Meta" {
		t.Fatalf("unexpected route report: %+v", report)
	}
	if len(report.Candidates) != 2 {
		t.Fatalf("expected two candidates, got %+v", report.Candidates)
	}
	if report.Candidates[0].InterfaceAlias != "以太网" || !report.Candidates[0].Recommended {
		t.Fatalf("expected physical ethernet to be first recommended candidate, got %+v", report.Candidates)
	}
	if report.Candidates[0].PublicIPv4 != "112.10.189.69:1142" {
		t.Fatalf("expected physical candidate public mapping, got %+v", report.Candidates[0])
	}
	if !report.Candidates[1].SuspectedProxy {
		t.Fatalf("expected Meta candidate to be marked as proxy, got %+v", report.Candidates[1])
	}
	if report.Candidates[1].PublicIPv4 != "172.105.240.197:43051" {
		t.Fatalf("expected proxy candidate public mapping, got %+v", report.Candidates[1])
	}
}

func TestDiagnosePeerReportsDERPAndStillListsEgressCandidates(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"tailscale ping --c 10 100.109.251.97": []byte("pong from desktop via DERP(hkg) in 43ms"),
		"Get-NetRoute -DestinationPrefix": []byte(`[
			{"InterfaceAlias":"以太网","InterfaceIndex":15,"NextHop":"192.168.1.1","RouteMetric":0,"InterfaceMetric":25,"InterfaceIP":"192.168.1.11"}
		]`),
		"tailscale netcheck --format json --bind-address 192.168.1.11": []byte(`{"UDP":true,"GlobalV4":"112.10.189.69:1142","GlobalV6":""}`),
	}}
	service := NewService(runner)

	report, err := service.DiagnosePeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != PathDERP || report.RouteType != RouteDERP {
		t.Fatalf("unexpected DERP report: %+v", report)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].PublicIPv4 != "112.10.189.69:1142" {
		t.Fatalf("expected DERP report to include egress candidates, got %+v", report.Candidates)
	}
}
