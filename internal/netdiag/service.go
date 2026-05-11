package netdiag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Service struct {
	runner Runner
}

func NewService(runner Runner) *Service {
	if runner == nil {
		runner = execRunner{}
	}
	return &Service{runner: runner}
}

func (s *Service) DiagnosePeer(ctx context.Context, peerTailscaleIP string) (PeerPathReport, error) {
	peerTailscaleIP = strings.TrimSpace(peerTailscaleIP)
	report := PeerPathReport{PeerTailscaleIP: peerTailscaleIP, Status: PathUnknown}
	if peerTailscaleIP == "" {
		report.Status = PathFailed
		report.Message = "缺少对端 Tailscale IP"
		return report, fmt.Errorf(report.Message)
	}

	raw, err := s.runner.Run(ctx, "tailscale", "ping", "--c", "10", peerTailscaleIP)
	if err != nil {
		report.Status = PathFailed
		report.Message = "Tailscale 路径检测失败：" + err.Error()
		return report, err
	}
	routeType, endpoint, latency := ParsePingRoute(raw)
	report.RouteType = routeType
	report.Endpoint = endpoint
	report.EndpointIP = EndpointIP(endpoint)
	report.Latency = latency
	candidates, candidatesErr := s.egressCandidates(ctx, report.EndpointIP)

	if routeType != RouteDirect {
		report.Status = ClassifyPath(routeType, latency, RouteInfo{})
		report.Candidates = candidates
		report.Message = pathStatusMessage(report.Status)
		if candidatesErr != nil {
			report.Message += "；读取公网出口失败：" + candidatesErr.Error()
		}
		return report, nil
	}

	current, err := s.currentRoute(ctx, report.EndpointIP)
	if err != nil {
		report.Status = PathFailed
		report.Message = "读取当前出口失败：" + err.Error()
		return report, err
	}
	if candidatesErr != nil {
		report.Status = PathFailed
		report.Message = "读取公网出口失败：" + candidatesErr.Error()
		return report, candidatesErr
	}
	report.CurrentRoute = current
	report.Candidates = candidates
	report.Status = ClassifyPath(routeType, latency, current)
	report.Message = pathStatusMessage(report.Status)
	return report, nil
}

func (s *Service) ApplyBypass(ctx context.Context, request BypassRequest) (ActiveBypass, error) {
	if err := validateBypassRequest(request); err != nil {
		return ActiveBypass{}, err
	}
	if _, err := s.runPowerShell(ctx, newRouteScript(request)); err != nil {
		return ActiveBypass{}, err
	}
	active := ActiveBypass{
		PeerTailscaleIP: request.PeerTailscaleIP,
		EndpointIP:      request.EndpointIP,
		AddressFamily:   endpointAddressFamily(request.EndpointIP),
		InterfaceIndex:  request.Candidate.InterfaceIndex,
		NextHop:         request.Candidate.NextHop,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.verifyBypass(ctx, request); err != nil {
		_ = s.ClearBypass(ctx, active)
		return ActiveBypass{}, err
	}
	return active, nil
}

func (s *Service) ClearBypass(ctx context.Context, bypass ActiveBypass) error {
	if err := validateActiveBypass(bypass); err != nil {
		return err
	}
	_, err := s.runPowerShell(ctx, removeRouteScript(bypass))
	return err
}

func (s *Service) currentRoute(ctx context.Context, endpointIP string) (RouteInfo, error) {
	if endpointIP == "" {
		return RouteInfo{}, fmt.Errorf("缺少 endpoint IP")
	}
	raw, err := s.runPowerShell(ctx, findRouteScript(endpointIP))
	if err != nil {
		return RouteInfo{}, err
	}
	routes, err := parseRouteInfos(raw)
	if err != nil {
		return RouteInfo{}, err
	}
	if len(routes) == 0 {
		return RouteInfo{}, fmt.Errorf("没有找到到 %s 的路由", endpointIP)
	}
	return routes[0], nil
}

func (s *Service) egressCandidates(ctx context.Context, endpointIP string) ([]EgressCandidate, error) {
	raw, err := s.runPowerShell(ctx, defaultRoutesScript())
	if err != nil {
		return nil, err
	}
	candidates, err := parseEgressCandidates(raw)
	if err != nil {
		return nil, err
	}
	s.enrichCandidatePublicMappings(ctx, candidates)
	return rankEgressCandidates(candidates, endpointIP), nil
}

func (s *Service) runPowerShell(ctx context.Context, script string) ([]byte, error) {
	return s.runner.Run(ctx, "powershell.exe", powershellArgs(script)...)
}

func powershellArgs(script string) []string {
	prefix := "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); " +
		"$OutputEncoding = [System.Text.UTF8Encoding]::new($false); "
	return []string{"-NoProfile", "-NonInteractive", "-Command", prefix + script}
}

func (s *Service) enrichCandidatePublicMappings(ctx context.Context, candidates []EgressCandidate) {
	for i := range candidates {
		if strings.TrimSpace(candidates[i].InterfaceIP) == "" {
			continue
		}
		report, err := s.netcheck(ctx, candidates[i].InterfaceIP)
		if err != nil {
			candidates[i].NetcheckError = err.Error()
			continue
		}
		candidates[i].PublicIPv4 = report.GlobalV4
		candidates[i].PublicIPv6 = report.GlobalV6
		candidates[i].UDP = report.UDP
	}
}

type netcheckReport struct {
	UDP      bool   `json:"UDP"`
	GlobalV4 string `json:"GlobalV4"`
	GlobalV6 string `json:"GlobalV6"`
}

func (s *Service) netcheck(ctx context.Context, bindAddress string) (netcheckReport, error) {
	raw, err := s.runner.Run(ctx, "tailscale", "netcheck", "--format", "json", "--bind-address", bindAddress)
	if err != nil {
		return netcheckReport{}, err
	}
	var report netcheckReport
	if err := json.Unmarshal(bytes.TrimSpace(stripNetcheckWarning(raw)), &report); err != nil {
		return netcheckReport{}, err
	}
	return report, nil
}

func stripNetcheckWarning(raw []byte) []byte {
	text := string(raw)
	if index := strings.Index(text, "\n# Warning:"); index != -1 {
		text = text[:index]
	}
	return []byte(text)
}

func (s *Service) verifyBypass(ctx context.Context, request BypassRequest) error {
	_, _ = s.runner.Run(ctx, "tailscale", "debug", "restun")
	raw, err := s.runner.Run(ctx, "tailscale", "ping", "--c", "10", request.PeerTailscaleIP)
	if err != nil {
		return fmt.Errorf("验证 Tailscale 直连失败：%w", err)
	}
	routeType, endpoint, latency := ParsePingRoute(raw)
	if routeType != RouteDirect {
		return fmt.Errorf("所选出口未建立直连，当前为 %s %s %s", routeType, endpoint, latency)
	}
	endpointIP := EndpointIP(endpoint)
	if endpointIP != request.EndpointIP {
		return fmt.Errorf("Tailscale endpoint 已变化为 %s，请重新检测网络路径", endpoint)
	}
	current, err := s.currentRoute(ctx, endpointIP)
	if err != nil {
		return fmt.Errorf("验证当前出口失败：%w", err)
	}
	if current.InterfaceIndex != request.Candidate.InterfaceIndex {
		return fmt.Errorf("所选出口未生效，当前仍走 %s", current.InterfaceAlias)
	}
	return nil
}

func findRouteScript(endpointIP string) string {
	return fmt.Sprintf("Find-NetRoute -RemoteIPAddress '%s' | Select-Object -First 1 InterfaceAlias,InterfaceIndex,NextHop,RouteMetric,InterfaceMetric,IPAddress,@{Name='AddressFamily';Expression={if ('%s' -like '*:*') {'IPv6'} else {'IPv4'}}} | ConvertTo-Json -Compress", endpointIP, endpointIP)
}

func defaultRoutesScript() string {
	return "$items = @(); " +
		"$items += Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Where-Object { $_.NextHop -and $_.NextHop -ne '0.0.0.0' } | ForEach-Object { " +
		"$iface = Get-NetIPInterface -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1; " +
		"$ip = Get-NetIPAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.IPAddress -notlike '169.254*' } | Select-Object -First 1; " +
		"[pscustomobject]@{AddressFamily='IPv4';InterfaceAlias=$_.InterfaceAlias;InterfaceIndex=$_.InterfaceIndex;NextHop=$_.NextHop;RouteMetric=$_.RouteMetric;InterfaceMetric=$iface.InterfaceMetric;InterfaceIP=$ip.IPAddress} " +
		"}; " +
		"$items += Get-NetRoute -DestinationPrefix '::/0' -ErrorAction SilentlyContinue | Where-Object { $_.NextHop -and $_.NextHop -ne '::' } | ForEach-Object { " +
		"$iface = Get-NetIPInterface -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv6 -ErrorAction SilentlyContinue | Select-Object -First 1; " +
		"$ip = Get-NetIPAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv6 -ErrorAction SilentlyContinue | Where-Object { $_.IPAddress -notlike 'fe80*' -and $_.AddressState -eq 'Preferred' } | Select-Object -First 1; " +
		"[pscustomobject]@{AddressFamily='IPv6';InterfaceAlias=$_.InterfaceAlias;InterfaceIndex=$_.InterfaceIndex;NextHop=$_.NextHop;RouteMetric=$_.RouteMetric;InterfaceMetric=$iface.InterfaceMetric;InterfaceIP=$ip.IPAddress} " +
		"}; " +
		"$items | ConvertTo-Json -Compress"
}

func newRouteScript(request BypassRequest) string {
	return fmt.Sprintf(
		"New-NetRoute -DestinationPrefix '%s' -InterfaceIndex %d -NextHop '%s' -PolicyStore ActiveStore",
		destinationPrefix(request.EndpointIP),
		request.Candidate.InterfaceIndex,
		request.Candidate.NextHop,
	)
}

func removeRouteScript(bypass ActiveBypass) string {
	return fmt.Sprintf(
		"Remove-NetRoute -DestinationPrefix '%s' -InterfaceIndex %d -NextHop '%s' -Confirm:$false",
		destinationPrefix(bypass.EndpointIP),
		bypass.InterfaceIndex,
		bypass.NextHop,
	)
}

func parseRouteInfos(raw []byte) ([]RouteInfo, error) {
	var payload []struct {
		InterfaceAlias string `json:"InterfaceAlias"`
		InterfaceIndex int    `json:"InterfaceIndex"`
		NextHop        string `json:"NextHop"`
		IPAddress      string `json:"IPAddress"`
		AddressFamily  string `json:"AddressFamily"`
	}
	if err := unmarshalOneOrMany(raw, &payload); err != nil {
		return nil, err
	}
	routes := make([]RouteInfo, 0, len(payload))
	for _, item := range payload {
		routes = append(routes, RouteInfo{
			InterfaceAlias: item.InterfaceAlias,
			InterfaceIndex: item.InterfaceIndex,
			NextHop:        item.NextHop,
			IPAddress:      item.IPAddress,
			AddressFamily:  normalizedAddressFamily(item.AddressFamily, item.NextHop, item.IPAddress),
		})
	}
	return routes, nil
}

func parseEgressCandidates(raw []byte) ([]EgressCandidate, error) {
	var payload []struct {
		InterfaceAlias  string `json:"InterfaceAlias"`
		InterfaceIndex  int    `json:"InterfaceIndex"`
		InterfaceIP     string `json:"InterfaceIP"`
		NextHop         string `json:"NextHop"`
		AddressFamily   string `json:"AddressFamily"`
		RouteMetric     int    `json:"RouteMetric"`
		InterfaceMetric int    `json:"InterfaceMetric"`
	}
	if err := unmarshalOneOrMany(raw, &payload); err != nil {
		return nil, err
	}
	candidates := make([]EgressCandidate, 0, len(payload))
	for _, item := range payload {
		alias := strings.TrimSpace(item.InterfaceAlias)
		if alias == "" || strings.EqualFold(alias, "Tailscale") || strings.Contains(strings.ToLower(alias), "loopback") {
			continue
		}
		nextHop := strings.TrimSpace(item.NextHop)
		if nextHop == "" || nextHop == "0.0.0.0" || nextHop == "::" {
			continue
		}
		addressFamily := normalizedAddressFamily(item.AddressFamily, nextHop, item.InterfaceIP)
		candidates = append(candidates, EgressCandidate{
			InterfaceAlias:  alias,
			InterfaceIndex:  item.InterfaceIndex,
			InterfaceIP:     item.InterfaceIP,
			NextHop:         nextHop,
			AddressFamily:   addressFamily,
			RouteMetric:     item.RouteMetric,
			InterfaceMetric: item.InterfaceMetric,
			SuspectedProxy:  IsSuspectedProxyInterface(alias),
		})
	}
	return candidates, nil
}

func rankEgressCandidates(candidates []EgressCandidate, endpointIP string) []EgressCandidate {
	ranked := append([]EgressCandidate(nil), candidates...)
	endpointFamily := endpointAddressFamily(endpointIP)
	sort.SliceStable(ranked, func(i, j int) bool {
		if endpointFamily != "" && ranked[i].AddressFamily != ranked[j].AddressFamily {
			return ranked[i].AddressFamily == endpointFamily
		}
		if ranked[i].SuspectedProxy != ranked[j].SuspectedProxy {
			return !ranked[i].SuspectedProxy
		}
		left := ranked[i].RouteMetric + ranked[i].InterfaceMetric
		right := ranked[j].RouteMetric + ranked[j].InterfaceMetric
		if left != right {
			return left < right
		}
		return ranked[i].InterfaceAlias < ranked[j].InterfaceAlias
	})
	for i := range ranked {
		ranked[i].Recommended = false
	}
	for i := range ranked {
		if endpointFamily != "" && ranked[i].AddressFamily != endpointFamily {
			continue
		}
		if !ranked[i].SuspectedProxy {
			ranked[i].Recommended = true
			break
		}
	}
	return ranked
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

func pathStatusMessage(status PathStatus) string {
	switch status {
	case PathDirectNormal:
		return "Tailscale 直连正常"
	case PathDirectTUNOptimized:
		return "Tailscale 低延迟直连，TUN 已接管但当前路径可用"
	case PathDirectProxy:
		return "Tailscale 已直连，但疑似被代理/TUN 接管"
	case PathDERP:
		return "Tailscale 当前走中继"
	case PathFailed:
		return "网络路径检测失败"
	default:
		return "网络路径未知"
	}
}

func validateBypassRequest(request BypassRequest) error {
	if !IsPublicEndpointIP(request.EndpointIP) {
		return fmt.Errorf("endpoint IP 不是可绕过的公网地址：%s", request.EndpointIP)
	}
	if request.Candidate.InterfaceIndex <= 0 {
		return fmt.Errorf("缺少公网出口接口")
	}
	if strings.TrimSpace(request.Candidate.NextHop) == "" {
		return fmt.Errorf("缺少公网出口网关")
	}
	endpointFamily := endpointAddressFamily(request.EndpointIP)
	if request.Candidate.AddressFamily != "" && request.Candidate.AddressFamily != endpointFamily {
		return fmt.Errorf("公网出口地址族不匹配，endpoint 为 %s，出口为 %s", endpointFamily, request.Candidate.AddressFamily)
	}
	return nil
}

func validateActiveBypass(bypass ActiveBypass) error {
	if !IsPublicEndpointIP(bypass.EndpointIP) {
		return fmt.Errorf("endpoint IP 不是可撤销的公网地址：%s", bypass.EndpointIP)
	}
	if bypass.InterfaceIndex <= 0 {
		return fmt.Errorf("缺少要撤销的接口")
	}
	if strings.TrimSpace(bypass.NextHop) == "" {
		return fmt.Errorf("缺少要撤销的网关")
	}
	return nil
}

func normalizedAddressFamily(values ...string) string {
	for _, value := range values {
		family := strings.TrimSpace(value)
		if family == AddressFamilyIPv4 || family == AddressFamilyIPv6 {
			return family
		}
		if detected := endpointAddressFamily(family); detected != "" {
			return detected
		}
	}
	return ""
}

func destinationPrefix(endpointIP string) string {
	if endpointAddressFamily(endpointIP) == AddressFamilyIPv6 {
		return endpointIP + "/128"
	}
	return endpointIP + "/32"
}
