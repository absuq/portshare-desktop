package netdiag

import (
	"context"
	"strings"
	"testing"
)

func TestApplyBypassAddsActiveStoreHostRoute(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"New-NetRoute":                   []byte(""),
		"tailscale debug restun":         []byte(""),
		"tailscale ping --c 10":          []byte("pong from desktop via 115.233.222.82:41641 in 25ms"),
		"Find-NetRoute -RemoteIPAddress": []byte(`{"InterfaceAlias":"以太网","InterfaceIndex":15,"NextHop":"192.168.1.1","IPAddress":"192.168.1.11"}`),
	}}
	service := NewService(runner)

	active, err := service.ApplyBypass(context.Background(), BypassRequest{
		PeerTailscaleIP: "100.109.251.97",
		EndpointIP:      "115.233.222.82",
		Candidate: EgressCandidate{
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if active.EndpointIP != "115.233.222.82" || active.InterfaceIndex != 15 || active.NextHop != "192.168.1.1" || active.CreatedAt.IsZero() {
		t.Fatalf("unexpected active bypass: %+v", active)
	}
	command := strings.Join(runner.commands, "\n")
	for _, want := range []string{"New-NetRoute", "115.233.222.82/32", "-InterfaceIndex 15", "-NextHop '192.168.1.1'", "-PolicyStore ActiveStore"} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected command to contain %q, got %s", want, command)
		}
	}
	if strings.Contains(command, "0.0.0.0/0") {
		t.Fatalf("route command must not change default route: %s", command)
	}
}

func TestApplyBypassAddsActiveStoreIPv6HostRoute(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"New-NetRoute":                   []byte(""),
		"tailscale debug restun":         []byte(""),
		"tailscale ping --c 10":          []byte("pong from desktop via [2401:b60:1b::1033]:13674 in 25ms"),
		"Find-NetRoute -RemoteIPAddress": []byte(`{"InterfaceAlias":"以太网","InterfaceIndex":15,"NextHop":"fe80::1","IPAddress":"2409:8a28:127d:e2f0:e431:c739:7833:d9b5","AddressFamily":"IPv6"}`),
	}}
	service := NewService(runner)

	active, err := service.ApplyBypass(context.Background(), BypassRequest{
		PeerTailscaleIP: "100.109.251.97",
		EndpointIP:      "2401:b60:1b::1033",
		Candidate: EgressCandidate{
			AddressFamily:  AddressFamilyIPv6,
			InterfaceIndex: 15,
			NextHop:        "fe80::1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if active.EndpointIP != "2401:b60:1b::1033" || active.AddressFamily != AddressFamilyIPv6 {
		t.Fatalf("unexpected active bypass: %+v", active)
	}
	command := strings.Join(runner.commands, "\n")
	for _, want := range []string{"New-NetRoute", "2401:b60:1b::1033/128", "-InterfaceIndex 15", "-NextHop 'fe80::1'", "-PolicyStore ActiveStore"} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected command to contain %q, got %s", want, command)
		}
	}
}

func TestApplyBypassRollsBackWhenRouteFallsBackToDERP(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"New-NetRoute":           []byte(""),
		"tailscale debug restun": []byte(""),
		"tailscale ping --c 10":  []byte("pong from desktop via DERP(sfo) in 341ms"),
		"Remove-NetRoute":        []byte(""),
	}}
	service := NewService(runner)

	_, err := service.ApplyBypass(context.Background(), BypassRequest{
		PeerTailscaleIP: "100.109.251.97",
		EndpointIP:      "115.233.222.82",
		Candidate: EgressCandidate{
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
		},
	})
	if err == nil {
		t.Fatal("expected DERP fallback to fail")
	}

	command := strings.Join(runner.commands, "\n")
	if !strings.Contains(command, "Remove-NetRoute") {
		t.Fatalf("expected failed bypass to roll back route, got %s", command)
	}
}

func TestApplyBypassRollsBackWhenSelectedInterfaceIsNotUsed(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"New-NetRoute":                   []byte(""),
		"tailscale debug restun":         []byte(""),
		"tailscale ping --c 10":          []byte("pong from desktop via 115.233.222.82:41641 in 25ms"),
		"Find-NetRoute -RemoteIPAddress": []byte(`{"InterfaceAlias":"Meta","InterfaceIndex":19,"NextHop":"198.18.0.2","IPAddress":"198.18.0.1"}`),
		"Remove-NetRoute":                []byte(""),
	}}
	service := NewService(runner)

	_, err := service.ApplyBypass(context.Background(), BypassRequest{
		PeerTailscaleIP: "100.109.251.97",
		EndpointIP:      "115.233.222.82",
		Candidate: EgressCandidate{
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
		},
	})
	if err == nil {
		t.Fatal("expected wrong interface to fail")
	}

	command := strings.Join(runner.commands, "\n")
	if !strings.Contains(command, "Remove-NetRoute") {
		t.Fatalf("expected failed bypass to roll back route, got %s", command)
	}
}

func TestClearBypassRemovesRecordedHostRoute(t *testing.T) {
	runner := &recordingRunner{outputs: map[string][]byte{
		"Remove-NetRoute": []byte(""),
	}}
	service := NewService(runner)

	err := service.ClearBypass(context.Background(), ActiveBypass{
		EndpointIP:     "115.233.222.82",
		InterfaceIndex: 15,
		NextHop:        "192.168.1.1",
	})
	if err != nil {
		t.Fatal(err)
	}

	command := strings.Join(runner.commands, "\n")
	for _, want := range []string{"Remove-NetRoute", "115.233.222.82/32", "-InterfaceIndex 15", "-NextHop '192.168.1.1'", "-Confirm:$false"} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected command to contain %q, got %s", want, command)
		}
	}
}

func TestApplyBypassRejectsNonPublicEndpoint(t *testing.T) {
	service := NewService(&recordingRunner{})

	_, err := service.ApplyBypass(context.Background(), BypassRequest{
		EndpointIP: "198.18.0.1",
		Candidate:  EgressCandidate{InterfaceIndex: 15, NextHop: "192.168.1.1"},
	})
	if err == nil {
		t.Fatal("expected non-public endpoint to be rejected")
	}
}
