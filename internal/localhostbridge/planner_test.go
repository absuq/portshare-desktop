package localhostbridge

import "testing"

func TestBuildPlanBridgesLoopbackOnlyPort(t *testing.T) {
	plans := BuildPlan(PlanInput{
		LocalTailscaleIP: "100.79.83.104",
		AllowedPeerIPs:   []string{"100.109.251.97"},
		Listeners: []ListeningPort{
			{Address: "127.0.0.1", Port: 18789},
		},
	})

	if len(plans) != 1 {
		t.Fatalf("expected one bridge plan, got %+v", plans)
	}
	if plans[0].ListenAddress != "100.79.83.104:18789" || plans[0].TargetAddress != "127.0.0.1:18789" {
		t.Fatalf("unexpected bridge plan: %+v", plans[0])
	}
	if len(plans[0].AllowedPeerIPs) != 1 || plans[0].AllowedPeerIPs[0] != "100.109.251.97" {
		t.Fatalf("unexpected allowed peers: %+v", plans[0].AllowedPeerIPs)
	}
}

func TestBuildPlanDoesNotBridgeWildcardListener(t *testing.T) {
	plans := BuildPlan(PlanInput{
		LocalTailscaleIP: "100.79.83.104",
		AllowedPeerIPs:   []string{"100.109.251.97"},
		Listeners: []ListeningPort{
			{Address: "127.0.0.1", Port: 52726},
			{Address: "0.0.0.0", Port: 52726},
		},
	})

	if len(plans) != 0 {
		t.Fatalf("expected wildcard listener to remain native, got %+v", plans)
	}
}

func TestBuildPlanDoesNotBridgeNativeTailscaleListener(t *testing.T) {
	plans := BuildPlan(PlanInput{
		LocalTailscaleIP: "100.79.83.104",
		AllowedPeerIPs:   []string{"100.109.251.97"},
		Listeners: []ListeningPort{
			{Address: "127.0.0.1", Port: 17890},
			{Address: "100.79.83.104", Port: 17890},
		},
	})

	if len(plans) != 0 {
		t.Fatalf("expected tailscale listener to remain native, got %+v", plans)
	}
}

func TestBuildPlanDoesNotBridgeWithoutTrustedPeers(t *testing.T) {
	plans := BuildPlan(PlanInput{
		LocalTailscaleIP: "100.79.83.104",
		Listeners: []ListeningPort{
			{Address: "127.0.0.1", Port: 18789},
		},
	})

	if len(plans) != 0 {
		t.Fatalf("expected no bridge without trusted peers, got %+v", plans)
	}
}
