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

func TestDiagnosePeerReportsIPv6DirectProxyRoute(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"tailscale ping --c 10 100.109.251.97": []byte("pong from desktop via [2401:b60:1b::1033]:13674 in 136ms"),
		"Find-NetRoute": []byte(`{
			"InterfaceAlias":"Meta",
			"InterfaceIndex":19,
			"NextHop":"fdfe:dcba:9876::2",
			"RouteMetric":0,
			"InterfaceMetric":0,
			"IPAddress":"fdfe:dcba:9876::1",
			"AddressFamily":"IPv6"
		}`),
		"Get-NetRoute -DestinationPrefix": []byte(`[
			{"InterfaceAlias":"Meta","InterfaceIndex":19,"NextHop":"198.18.0.2","RouteMetric":0,"InterfaceMetric":0,"InterfaceIP":"198.18.0.1","AddressFamily":"IPv4"},
			{"InterfaceAlias":"以太网","InterfaceIndex":15,"NextHop":"192.168.1.1","RouteMetric":0,"InterfaceMetric":25,"InterfaceIP":"192.168.1.11","AddressFamily":"IPv4"},
			{"InterfaceAlias":"Meta","InterfaceIndex":19,"NextHop":"fdfe:dcba:9876::2","RouteMetric":0,"InterfaceMetric":0,"InterfaceIP":"fdfe:dcba:9876::1","AddressFamily":"IPv6"},
			{"InterfaceAlias":"以太网","InterfaceIndex":15,"NextHop":"fe80::1","RouteMetric":256,"InterfaceMetric":25,"InterfaceIP":"2409:8a28:127d:e2f0:e431:c739:7833:d9b5","AddressFamily":"IPv6"}
		]`),
		"tailscale netcheck --format json --bind-address 198.18.0.1":                              []byte(`{"UDP":true,"GlobalV4":"157.254.20.171:6223","GlobalV6":""}`),
		"tailscale netcheck --format json --bind-address 192.168.1.11":                            []byte(`{"UDP":true,"GlobalV4":"112.10.189.69:1142","GlobalV6":""}`),
		"tailscale netcheck --format json --bind-address fdfe:dcba:9876::1":                       []byte(`{"UDP":true,"GlobalV4":"","GlobalV6":"[2401:b60:1b::1017]:18585"}`),
		"tailscale netcheck --format json --bind-address 2409:8a28:127d:e2f0:e431:c739:7833:d9b5": []byte(`{"UDP":true,"GlobalV4":"","GlobalV6":"[2409:8a28:127d:e2f0::100]:41641"}`),
	}}
	service := NewService(runner)

	report, err := service.DiagnosePeer(context.Background(), "100.109.251.97")
	if err != nil {
		t.Fatal(err)
	}

	if report.EndpointIP != "2401:b60:1b::1033" || report.CurrentRoute.AddressFamily != AddressFamilyIPv6 {
		t.Fatalf("unexpected IPv6 endpoint report: %+v", report)
	}
	if report.Status != PathDirectProxy {
		t.Fatalf("expected IPv6 direct proxy path, got %+v", report)
	}
	if len(report.Candidates) != 4 {
		t.Fatalf("expected four candidates, got %+v", report.Candidates)
	}
	if report.Candidates[0].InterfaceAlias != "以太网" || report.Candidates[0].AddressFamily != AddressFamilyIPv6 || !report.Candidates[0].Recommended {
		t.Fatalf("expected physical IPv6 candidate to be recommended first, got %+v", report.Candidates)
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

func TestServiceReprobeRunsRestunThenRebind(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"tailscale debug restun": []byte("ok"),
		"tailscale debug rebind": []byte("ok"),
	}}
	service := NewService(runner)

	result := service.Reprobe(context.Background(), ReprobeRequest{Restun: true, Rebind: true})

	if !result.RestunAttempted || !result.RebindAttempted || result.RestunError != "" || result.RebindError != "" {
		t.Fatalf("unexpected reprobe result: %+v", result)
	}
	command := strings.Join(runner.commands, "\n")
	restun := strings.Index(command, "tailscale debug restun")
	rebind := strings.Index(command, "tailscale debug rebind")
	if restun == -1 || rebind == -1 || restun > rebind {
		t.Fatalf("expected restun before rebind, got %s", command)
	}
}

func TestServiceReprobeRecordsDebugCommandFailures(t *testing.T) {
	service := NewService(&recordingRunner{})

	result := service.Reprobe(context.Background(), ReprobeRequest{Restun: true, Rebind: true})

	if !result.RestunAttempted || !result.RebindAttempted {
		t.Fatalf("expected both commands to be attempted, got %+v", result)
	}
	if result.RestunError == "" || result.RebindError == "" {
		t.Fatalf("expected errors to be recorded, got %+v", result)
	}
}
