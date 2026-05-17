package linkguardian

import (
	"strings"
	"testing"
	"time"

	"github.com/absuq/portshare-desktop/internal/netdiag"
)

func TestEvaluateKeepsLowLatencyDirectWithoutBypass(t *testing.T) {
	report := netdiag.PeerPathReport{
		Status:       netdiag.PathDirectTUNOptimized,
		RouteType:    netdiag.RouteDirect,
		Endpoint:     "115.233.222.82:52477",
		EndpointIP:   "115.233.222.82",
		Latency:      "15ms",
		CurrentRoute: netdiag.RouteInfo{InterfaceAlias: "Meta", NextHop: "198.18.0.2"},
	}

	decision := Evaluate(EvaluateInput{
		Path:          report,
		AutoBypass:    true,
		LatestLatency: 23 * time.Millisecond,
	})

	if decision.Status != StatusOptimized || decision.Action != ActionWatch {
		t.Fatalf("expected low latency direct to watch, got %+v", decision)
	}
	if decision.Candidate.InterfaceIndex != 0 {
		t.Fatalf("expected no bypass candidate, got %+v", decision.Candidate)
	}
}

func TestEvaluateAppliesBypassForHighLatencyTUNDirect(t *testing.T) {
	report := netdiag.PeerPathReport{
		PeerTailscaleIP: "100.109.251.97",
		Status:          netdiag.PathDirectProxy,
		RouteType:       netdiag.RouteDirect,
		Endpoint:        "115.233.222.82:41641",
		EndpointIP:      "115.233.222.82",
		Latency:         "249ms",
		CurrentRoute:    netdiag.RouteInfo{InterfaceAlias: "Meta", NextHop: "198.18.0.2"},
		Candidates: []netdiag.EgressCandidate{{
			InterfaceAlias: "Meta",
			InterfaceIndex: 12,
			NextHop:        "198.18.0.2",
			SuspectedProxy: true,
		}, {
			InterfaceAlias: "Ethernet",
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
			Recommended:    true,
		}},
	}

	decision := Evaluate(EvaluateInput{Path: report, AutoBypass: true})

	if decision.Status != StatusBypassReady || decision.Action != ActionApplyBypass {
		t.Fatalf("expected high latency TUN direct to apply bypass, got %+v", decision)
	}
	if decision.Candidate.InterfaceIndex != 15 || decision.Candidate.NextHop != "192.168.1.1" {
		t.Fatalf("expected recommended physical candidate, got %+v", decision.Candidate)
	}
}

func TestEvaluateOnlyWarnsWhenAutoBypassDisabled(t *testing.T) {
	report := netdiag.PeerPathReport{
		Status:     netdiag.PathDirectProxy,
		EndpointIP: "115.233.222.82",
		Latency:    "249ms",
		Candidates: []netdiag.EgressCandidate{{
			InterfaceAlias: "Ethernet",
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
			Recommended:    true,
		}},
	}

	decision := Evaluate(EvaluateInput{Path: report, AutoBypass: false})

	if decision.Status != StatusBypassReady || decision.Action != ActionWatch {
		t.Fatalf("expected disabled auto bypass to only watch, got %+v", decision)
	}
}

func TestEvaluateReprobesRelayPaths(t *testing.T) {
	decision := Evaluate(EvaluateInput{
		Path: netdiag.PeerPathReport{
			Status:    netdiag.PathDERP,
			RouteType: netdiag.RouteDERP,
			Endpoint:  "DERP(tok)",
			Latency:   "180ms",
		},
		AutoBypass: true,
	})

	if decision.Status != StatusRelay || decision.Action != ActionReprobe {
		t.Fatalf("expected DERP to reprobe, got %+v", decision)
	}
}

func TestEvaluateRequestsRollbackWhenActiveEndpointChanges(t *testing.T) {
	decision := Evaluate(EvaluateInput{
		Path: netdiag.PeerPathReport{
			Status:     netdiag.PathDirectNormal,
			EndpointIP: "115.233.222.99",
			Latency:    "18ms",
		},
		ActiveBypass: netdiag.ActiveBypass{
			EndpointIP:     "115.233.222.82",
			InterfaceIndex: 15,
			NextHop:        "192.168.1.1",
		},
		HasActiveBypass: true,
		AutoBypass:      true,
	})

	if decision.Status != StatusRollback || decision.Action != ActionClearBypass {
		t.Fatalf("expected endpoint change to clear active bypass, got %+v", decision)
	}
}

func TestEvaluateMentionsLatestLatencySample(t *testing.T) {
	decision := Evaluate(EvaluateInput{
		Path: netdiag.PeerPathReport{
			Status:  netdiag.PathDirectNormal,
			Latency: "15ms",
		},
		LatestLatency: 23 * time.Millisecond,
	})

	if !strings.Contains(decision.Message, "23ms") {
		t.Fatalf("expected message to include reused latency sample, got %q", decision.Message)
	}
}
