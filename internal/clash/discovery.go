package clash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/absuq/portshare-desktop/internal/netdiag"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Service struct {
	runner        Runner
	roots         []string
	client        ControllerClient
	verifier      TailscaleVerifier
	previousGroup string
	previousNode  string
}

type TailscaleVerifier interface {
	VerifyTailscaleDirect(context.Context, string) (RouteCheck, error)
}

func NewService(runner Runner, roots []string) *Service {
	if runner == nil {
		runner = execRunner{}
	}
	return &Service{runner: runner, roots: roots, verifier: runnerVerifier{runner: runner}}
}

func (s *Service) Discover(ctx context.Context) (DiscoveryReport, error) {
	cfg, err := s.discoverConfig()
	if err != nil {
		return DiscoveryReport{}, err
	}
	report := DiscoveryReport{Config: cfg}
	report.ProxyPorts = proxyPorts(cfg)
	report.Control = controlEndpoint(cfg)
	if s.client == nil && report.Control.Kind != ControlNone {
		s.client = controllerForEndpoint(report.Control)
	}

	adapters, err := s.tunInterfaces(ctx)
	if err == nil {
		report.TUNInterfaces = adapters
	}
	return report, nil
}

func (s *Service) RefreshNodes(ctx context.Context) (DiscoveryReport, error) {
	report, err := s.Discover(ctx)
	if err != nil {
		return report, err
	}
	client, err := s.controller(ctx)
	if err != nil {
		return report, err
	}
	snapshot, err := client.Proxies(ctx)
	if err != nil {
		return report, err
	}
	report.Nodes = nodesFromSnapshot(ctx, client, snapshot)
	return report, nil
}

func (s *Service) ApplyNode(ctx context.Context, request ApplyRequest) (ApplyResult, error) {
	client, err := s.controller(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	if strings.TrimSpace(request.PeerTailscaleIP) == "" {
		return ApplyResult{}, fmt.Errorf("缺少对端 Tailscale IP")
	}
	if strings.TrimSpace(request.GroupName) == "" || strings.TrimSpace(request.NodeName) == "" {
		return ApplyResult{}, fmt.Errorf("缺少 Clash/Mihomo 节点选择")
	}
	if err := client.Select(ctx, request.GroupName, request.NodeName); err != nil {
		return ApplyResult{}, err
	}
	s.previousGroup = request.GroupName
	s.previousNode = request.PreviousNode

	check, err := s.verifier.VerifyTailscaleDirect(ctx, request.PeerTailscaleIP)
	result := ApplyResult{
		GroupName:    request.GroupName,
		NodeName:     request.NodeName,
		PreviousNode: request.PreviousNode,
		RouteType:    check.RouteType,
		Endpoint:     check.Endpoint,
		Latency:      check.Latency,
	}
	if err != nil {
		s.restorePreviousBestEffort(ctx, client)
		result.RestoredPrevious = request.PreviousNode != ""
		return result, err
	}
	if check.RouteType != netdiag.RouteDirect {
		s.restorePreviousBestEffort(ctx, client)
		result.RestoredPrevious = request.PreviousNode != ""
		return result, fmt.Errorf("Tailscale 未建立 direct，当前为 %s %s %s", check.RouteType, check.Endpoint, check.Latency)
	}
	result.Improved = true
	return result, nil
}

func (s *Service) RestoreNode(ctx context.Context) error {
	client, err := s.controller(ctx)
	if err != nil {
		return err
	}
	if s.previousGroup == "" || s.previousNode == "" {
		return fmt.Errorf("没有可恢复的 Clash/Mihomo 节点")
	}
	return client.Select(ctx, s.previousGroup, s.previousNode)
}

func (s *Service) controller(ctx context.Context) (ControllerClient, error) {
	if s.client != nil {
		return s.client, nil
	}
	report, err := s.Discover(ctx)
	if err != nil {
		return nil, err
	}
	if report.Control.Kind == ControlNone {
		return nil, fmt.Errorf("未发现 Clash/Mihomo 控制接口")
	}
	s.client = controllerForEndpoint(report.Control)
	return s.client, nil
}

func controllerForEndpoint(endpoint ControlEndpoint) ControllerClient {
	switch endpoint.Kind {
	case ControlNamedPipe:
		return NewPipeController(endpoint.Address, endpoint.Secret)
	case ControlHTTP:
		return NewHTTPController(endpoint.Address, endpoint.Secret)
	default:
		return nil
	}
}

func nodesFromSnapshot(ctx context.Context, client ControllerClient, snapshot ProxySnapshot) []ProxyNode {
	var nodes []ProxyNode
	for _, group := range snapshot.Groups {
		for _, option := range group.Options {
			delay := option.Delay
			if refreshed, err := client.Delay(ctx, option.Name, defaultDelayTestURL, defaultDelayTimeoutMS); err == nil && refreshed > 0 {
				delay = refreshed
			}
			nodes = append(nodes, ProxyNode{
				GroupName: group.Name,
				Name:      option.Name,
				Type:      option.Type,
				Region:    InferRegion(option.Name),
				Current:   option.Name == group.Now,
				Delay:     delay,
			})
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Current != nodes[j].Current {
			return nodes[i].Current
		}
		if nodes[i].Delay != nodes[j].Delay {
			if nodes[i].Delay == 0 {
				return false
			}
			if nodes[j].Delay == 0 {
				return true
			}
			return nodes[i].Delay < nodes[j].Delay
		}
		return nodes[i].Name < nodes[j].Name
	})
	return nodes
}

func (s *Service) restorePreviousBestEffort(ctx context.Context, client ControllerClient) {
	if s.previousGroup == "" || s.previousNode == "" {
		return
	}
	_ = client.Select(ctx, s.previousGroup, s.previousNode)
}

type runnerVerifier struct {
	runner Runner
}

func (v runnerVerifier) VerifyTailscaleDirect(ctx context.Context, peerIP string) (RouteCheck, error) {
	_, _ = v.runner.Run(ctx, "tailscale", "debug", "restun")
	_, _ = v.runner.Run(ctx, "tailscale", "debug", "rebind")
	raw, err := v.runner.Run(ctx, "tailscale", "ping", "--c", "10", peerIP)
	if err != nil {
		return RouteCheck{}, err
	}
	routeType, endpoint, latency := netdiag.ParsePingRoute(raw)
	return RouteCheck{RouteType: routeType, Endpoint: endpoint, Latency: latency}, nil
}

func (s *Service) discoverConfig() (ClashConfig, error) {
	for _, path := range s.configCandidates() {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		cfg, err := ParseConfigYAML(raw)
		if err != nil {
			continue
		}
		cfg.SourcePath = path
		return cfg, nil
	}
	return ClashConfig{}, fmt.Errorf("未找到 Clash/Mihomo 配置")
}

func (s *Service) configCandidates() []string {
	roots := s.roots
	if len(roots) == 0 {
		roots = defaultConfigRoots()
	}
	var paths []string
	for _, root := range roots {
		for _, name := range []string{"clash-verge.yaml", "clash-verge-check.yaml", "config.yaml"} {
			paths = append(paths, filepath.Join(root, name))
		}
		profiles, _ := filepath.Glob(filepath.Join(root, "profiles", "*.yaml"))
		sort.Strings(profiles)
		paths = append(paths, profiles...)
	}
	return paths
}

func defaultConfigRoots() []string {
	var roots []string
	for _, env := range []string{"APPDATA", "LOCALAPPDATA"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		roots = append(roots, filepath.Join(base, "io.github.clash-verge-rev.clash-verge-rev"))
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		roots = append(roots, filepath.Join(appData, "clash_win"))
	}
	return roots
}

func proxyPorts(cfg ClashConfig) []ProxyPort {
	var ports []ProxyPort
	if cfg.MixedPort > 0 {
		ports = append(ports, ProxyPort{Kind: "mixed", Port: cfg.MixedPort})
	}
	if cfg.SocksPort > 0 {
		ports = append(ports, ProxyPort{Kind: "socks", Port: cfg.SocksPort})
	}
	if cfg.HTTPPort > 0 {
		ports = append(ports, ProxyPort{Kind: "http", Port: cfg.HTTPPort})
	}
	return ports
}

func controlEndpoint(cfg ClashConfig) ControlEndpoint {
	if strings.TrimSpace(cfg.ExternalControllerPipe) != "" {
		return ControlEndpoint{Kind: ControlNamedPipe, Address: strings.TrimSpace(cfg.ExternalControllerPipe), Secret: cfg.Secret}
	}
	controller := strings.TrimSpace(cfg.ExternalController)
	if controller == "" {
		return ControlEndpoint{Secret: cfg.Secret}
	}
	if !strings.Contains(controller, "://") {
		controller = "http://" + controller
	}
	if parsed, err := url.Parse(controller); err == nil && parsed.Host != "" {
		return ControlEndpoint{Kind: ControlHTTP, Address: strings.TrimRight(controller, "/"), Secret: cfg.Secret}
	}
	return ControlEndpoint{Secret: cfg.Secret}
}

func (s *Service) tunInterfaces(ctx context.Context) ([]TUNInterface, error) {
	raw, err := s.runPowerShell(ctx, adapterScript())
	if err != nil {
		return nil, err
	}
	var payload []struct {
		Name                 string `json:"Name"`
		InterfaceDescription string `json:"InterfaceDescription"`
		IfIndex              int    `json:"ifIndex"`
		Status               string `json:"Status"`
	}
	if err := unmarshalOneOrMany(raw, &payload); err != nil {
		return nil, err
	}
	var result []TUNInterface
	for _, item := range payload {
		if !isTUNLike(item.Name, item.InterfaceDescription) {
			continue
		}
		result = append(result, TUNInterface{
			Name:        item.Name,
			Description: item.InterfaceDescription,
			Index:       item.IfIndex,
			Status:      item.Status,
		})
	}
	return result, nil
}

func isTUNLike(name, description string) bool {
	value := strings.ToLower(name + " " + description)
	for _, marker := range []string{"meta", "mihomo", "clash", "tun", "sing-box", "proxy"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func (s *Service) runPowerShell(ctx context.Context, script string) ([]byte, error) {
	return s.runner.Run(ctx, "powershell.exe", powershellArgs(script)...)
}

func powershellArgs(script string) []string {
	prefix := "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); " +
		"$OutputEncoding = [System.Text.UTF8Encoding]::new($false); "
	return []string{"-NoProfile", "-NonInteractive", "-Command", prefix + script}
}

func adapterScript() string {
	return "Get-NetAdapter | Select-Object Name,InterfaceDescription,ifIndex,Status | ConvertTo-Json -Compress"
}

func listenScript() string {
	return "Get-NetTCPConnection -State Listen | Select-Object LocalAddress,LocalPort,OwningProcess | ConvertTo-Json -Compress"
}

func parsePort(value string) int {
	port, _ := strconv.Atoi(strings.TrimSpace(value))
	return port
}

func unmarshalOneOrMany[T any](raw []byte, out *[]T) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '[' {
		return json.Unmarshal(raw, out)
	}
	var single T
	if err := json.Unmarshal(raw, &single); err != nil {
		return err
	}
	*out = []T{single}
	return nil
}
