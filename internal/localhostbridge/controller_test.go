package localhostbridge

import (
	"context"
	"reflect"
	"testing"
)

type fakeScanner struct {
	listeners []ListeningPort
	err       error
}

func (s *fakeScanner) Scan(context.Context) ([]ListeningPort, error) {
	return append([]ListeningPort(nil), s.listeners...), s.err
}

type fakeBridgeRunner struct {
	plan    BridgePlan
	started bool
	closed  bool
}

func (b *fakeBridgeRunner) Start(context.Context) error {
	b.started = true
	return nil
}

func (b *fakeBridgeRunner) Close() error {
	b.closed = true
	return nil
}

func TestControllerRefreshStartsBridgeForLoopbackPort(t *testing.T) {
	scanner := &fakeScanner{listeners: []ListeningPort{{Address: "127.0.0.1", Port: 18789}}}
	created := map[int]*fakeBridgeRunner{}
	controller := NewController(Config{
		Scanner:          scanner,
		LocalTailscaleIP: "100.79.83.104",
		AllowedPeerIPs:   []string{"100.109.251.97"},
		NewBridge: func(plan BridgePlan) BridgeRunner {
			bridge := &fakeBridgeRunner{plan: plan}
			created[plan.Port] = bridge
			return bridge
		},
	})

	if err := controller.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	bridge := created[18789]
	if bridge == nil || !bridge.started {
		t.Fatalf("expected bridge to be started, got %+v", bridge)
	}
	if !reflect.DeepEqual(controller.ActivePorts(), []int{18789}) {
		t.Fatalf("unexpected active ports: %+v", controller.ActivePorts())
	}
}

func TestControllerRefreshStopsBridgeWhenPortDisappears(t *testing.T) {
	scanner := &fakeScanner{listeners: []ListeningPort{{Address: "127.0.0.1", Port: 18789}}}
	var bridge *fakeBridgeRunner
	controller := NewController(Config{
		Scanner:          scanner,
		LocalTailscaleIP: "100.79.83.104",
		AllowedPeerIPs:   []string{"100.109.251.97"},
		NewBridge: func(plan BridgePlan) BridgeRunner {
			bridge = &fakeBridgeRunner{plan: plan}
			return bridge
		},
	})

	if err := controller.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	scanner.listeners = nil
	if err := controller.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	if bridge == nil || !bridge.closed {
		t.Fatalf("expected bridge to be closed, got %+v", bridge)
	}
	if len(controller.ActivePorts()) != 0 {
		t.Fatalf("expected no active ports, got %+v", controller.ActivePorts())
	}
}

func TestControllerSetAllowedPeersRefreshesExistingBridgePlan(t *testing.T) {
	scanner := &fakeScanner{listeners: []ListeningPort{{Address: "127.0.0.1", Port: 18789}}}
	var closed []*fakeBridgeRunner
	created := []*fakeBridgeRunner{}
	controller := NewController(Config{
		Scanner:          scanner,
		LocalTailscaleIP: "100.79.83.104",
		AllowedPeerIPs:   []string{"100.109.251.97"},
		NewBridge: func(plan BridgePlan) BridgeRunner {
			bridge := &fakeBridgeRunner{plan: plan}
			created = append(created, bridge)
			return bridge
		},
	})

	if err := controller.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	controller.SetAllowedPeers([]string{"100.109.251.97", "100.109.251.98"})
	if err := controller.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, bridge := range created {
		if bridge.closed {
			closed = append(closed, bridge)
		}
	}
	if len(created) != 2 || len(closed) != 1 {
		t.Fatalf("expected bridge to restart with new peers, created=%+v closed=%+v", created, closed)
	}
	if !reflect.DeepEqual(created[1].plan.AllowedPeerIPs, []string{"100.109.251.97", "100.109.251.98"}) {
		t.Fatalf("unexpected refreshed allowed peers: %+v", created[1].plan.AllowedPeerIPs)
	}
}
