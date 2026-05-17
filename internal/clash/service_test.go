package clash

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

type fakeController struct {
	snapshot  ProxySnapshot
	delays    map[string]time.Duration
	selected  []string
	selectErr error
}

func (f *fakeController) Version(context.Context) (Version, error) {
	return Version{Version: "fake"}, nil
}

func (f *fakeController) Proxies(context.Context) (ProxySnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeController) Delay(_ context.Context, proxyName string, _ string, _ int) (time.Duration, error) {
	if delay, ok := f.delays[proxyName]; ok {
		return delay, nil
	}
	return 0, fmt.Errorf("missing delay for %s", proxyName)
}

func (f *fakeController) Select(_ context.Context, groupName string, proxyName string) error {
	if f.selectErr != nil {
		return f.selectErr
	}
	f.selected = append(f.selected, groupName+"="+proxyName)
	return nil
}

type fakeVerifier struct {
	result RouteCheck
	err    error
}

func (f fakeVerifier) VerifyTailscaleDirect(context.Context, string) (RouteCheck, error) {
	return f.result, f.err
}

func TestRefreshNodesReturnsRegionsCurrentAndDelays(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "clash-verge.yaml"), `
external-controller: 127.0.0.1:9097
`)
	controller := &fakeController{
		snapshot: ProxySnapshot{Groups: []ProxyGroup{{
			Name: "GLOBAL",
			Now:  "上海 01",
			Options: []ProxyOption{
				{Name: "上海 01", Type: "ss"},
				{Name: "杭州 01", Type: "vmess"},
			},
		}}},
		delays: map[string]time.Duration{
			"上海 01": 23 * time.Millisecond,
			"杭州 01": 35 * time.Millisecond,
		},
	}
	service := NewService(&fakeRunner{}, []string{root})
	service.client = controller

	report, err := service.RefreshNodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Nodes) != 2 {
		t.Fatalf("expected two nodes, got %+v", report.Nodes)
	}
	if !report.Nodes[0].Current || report.Nodes[0].Region != "上海" || report.Nodes[0].Delay != 23*time.Millisecond {
		t.Fatalf("unexpected first node: %+v", report.Nodes[0])
	}
	if report.Nodes[1].Region != "杭州" || report.Nodes[1].Delay != 35*time.Millisecond {
		t.Fatalf("unexpected second node: %+v", report.Nodes[1])
	}
}

func TestApplyNodeSelectsTargetAndVerifiesDirect(t *testing.T) {
	controller := &fakeController{}
	service := NewService(&fakeRunner{}, nil)
	service.client = controller
	service.verifier = fakeVerifier{result: RouteCheck{RouteType: "direct", Endpoint: "115.233.222.82:41641", Latency: "25ms"}}

	result, err := service.ApplyNode(context.Background(), ApplyRequest{
		PeerTailscaleIP: "100.109.251.97",
		GroupName:       "GLOBAL",
		NodeName:        "上海 01",
		PreviousNode:    "杭州 01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.RouteType != "direct" || result.Latency != "25ms" {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if len(controller.selected) != 1 || controller.selected[0] != "GLOBAL=上海 01" {
		t.Fatalf("expected target selection, got %+v", controller.selected)
	}
}

func TestApplyNodeRestoresPreviousWhenVerifierReportsDERP(t *testing.T) {
	controller := &fakeController{}
	service := NewService(&fakeRunner{}, nil)
	service.client = controller
	service.verifier = fakeVerifier{result: RouteCheck{RouteType: "derp", Endpoint: "DERP(sfo)", Latency: "341ms"}}

	_, err := service.ApplyNode(context.Background(), ApplyRequest{
		PeerTailscaleIP: "100.109.251.97",
		GroupName:       "GLOBAL",
		NodeName:        "上海 01",
		PreviousNode:    "杭州 01",
	})
	if err == nil {
		t.Fatal("expected DERP verification to fail")
	}
	if len(controller.selected) != 2 || controller.selected[1] != "GLOBAL=杭州 01" {
		t.Fatalf("expected previous node restore, got %+v", controller.selected)
	}
}

func TestRestoreNodeSelectsPreviousNode(t *testing.T) {
	controller := &fakeController{}
	service := NewService(&fakeRunner{}, nil)
	service.client = controller
	service.previousGroup = "GLOBAL"
	service.previousNode = "杭州 01"

	if err := service.RestoreNode(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(controller.selected) != 1 || controller.selected[0] != "GLOBAL=杭州 01" {
		t.Fatalf("expected previous node restore, got %+v", controller.selected)
	}
}
