package tailscale

import (
	"context"
	"fmt"
	"net"
	"strings"
)

type DiagnosticCode string

const (
	CodeOK                   DiagnosticCode = "ok"
	CodeTailscaleUnavailable DiagnosticCode = "tailscale.unavailable"
	CodeTailscaleStopped     DiagnosticCode = "tailscale.stopped"
	CodeNoTailscaleIP        DiagnosticCode = "tailscale.no_ip"
	CodePeerUnreachable      DiagnosticCode = "peer.unreachable"
	CodeDNSNotAccepted       DiagnosticCode = "dns.not_accepted"
)

type ReadyReport struct {
	Ready      bool
	Code       DiagnosticCode
	Message    string
	FixCommand string
	Status     Status
}

type RouteType string

const (
	RouteUnknown   RouteType = "unknown"
	RouteDirect    RouteType = "direct"
	RouteDERP      RouteType = "derp"
	RoutePeerRelay RouteType = "peer-relay"
)

type PeerRoute struct {
	Type    RouteType
	Via     string
	Latency string
	Raw     string
}

type Client struct {
	runner Runner
}

func NewClient(runner Runner) Client {
	if runner == nil {
		runner = ExecRunner{}
	}
	return Client{runner: runner}
}

func (c Client) CheckReady(ctx context.Context) ReadyReport {
	raw, err := c.runner.Run(ctx, "tailscale", "status", "--json")
	if err != nil {
		return ReadyReport{
			Code:    CodeTailscaleUnavailable,
			Message: fmt.Sprintf("未能运行 tailscale 命令，请确认 Tailscale 已安装并在 PATH 中：%v", err),
		}
	}

	status, err := ParseStatus(raw)
	if err != nil {
		return ReadyReport{
			Code:    CodeTailscaleUnavailable,
			Message: fmt.Sprintf("无法读取 Tailscale 状态，请确认 tailscale status --json 可正常输出：%v", err),
		}
	}
	if status.BackendState != "Running" {
		return ReadyReport{
			Code:       CodeTailscaleStopped,
			Message:    "Tailscale 当前未运行，请先启动或登录 Tailscale。",
			FixCommand: "tailscale up",
			Status:     status,
		}
	}
	if status.LocalIPv4 == "" {
		return ReadyReport{
			Code:    CodeNoTailscaleIP,
			Message: "Tailscale 未返回本机 IPv4 地址，请检查网络连接或重新登录。",
			Status:  status,
		}
	}
	if !status.MagicDNSEnabled {
		return ReadyReport{
			Code:       CodeDNSNotAccepted,
			Message:    "Tailscale DNS 未启用，请接受 Tailscale DNS 设置后重试。",
			FixCommand: "tailscale set --accept-dns=true",
			Status:     status,
		}
	}

	return ReadyReport{
		Ready:   true,
		Code:    CodeOK,
		Message: "Tailscale 已就绪。",
		Status:  status,
	}
}

func (c Client) PingPeer(ctx context.Context, peer string) (PeerRoute, error) {
	raw, err := c.runner.Run(ctx, "tailscale", "ping", peer)
	output := string(raw)
	route := PeerRoute{
		Type: RouteUnknown,
		Raw:  output,
	}
	if err != nil {
		return route, err
	}

	var peerRelayRoute PeerRoute
	var hasPeerRelayRoute bool
	var derpRoute PeerRoute
	var hasDERPRoute bool
	for _, line := range strings.Split(output, "\n") {
		candidate, ok := parsePingLine(line)
		if !ok {
			continue
		}
		candidate.Raw = output
		switch candidate.Type {
		case RouteDirect:
			return candidate, nil
		case RoutePeerRelay:
			if !hasPeerRelayRoute {
				peerRelayRoute = candidate
				hasPeerRelayRoute = true
			}
		case RouteDERP:
			if !hasDERPRoute {
				derpRoute = candidate
				hasDERPRoute = true
			}
		}
	}
	if hasPeerRelayRoute {
		return peerRelayRoute, nil
	}
	if hasDERPRoute {
		return derpRoute, nil
	}
	return route, nil
}

func parsePingLine(line string) (PeerRoute, bool) {
	line = strings.TrimSpace(line)
	const marker = " via "
	index := strings.Index(line, marker)
	if index == -1 {
		return PeerRoute{}, false
	}
	rest := line[index+len(marker):]
	if end := strings.Index(rest, " in "); end != -1 {
		rest = rest[:end]
	} else {
		return PeerRoute{}, false
	}
	via := strings.TrimSpace(rest)
	if via == "" {
		return PeerRoute{}, false
	}
	route := PeerRoute{
		Type:    routeTypeForVia(via),
		Via:     via,
		Latency: extractLatency(line),
	}
	return route, route.Type != RouteUnknown
}

func routeTypeForVia(via string) RouteType {
	switch {
	case strings.HasPrefix(via, "DERP("):
		return RouteDERP
	case strings.HasPrefix(via, "peer-relay("):
		return RoutePeerRelay
	case via == "ICMP" || via == "TSMP":
		return RouteUnknown
	case isDirectEndpoint(via):
		return RouteDirect
	default:
		return RouteUnknown
	}
}

func isDirectEndpoint(via string) bool {
	host, port, err := net.SplitHostPort(via)
	return err == nil && host != "" && port != ""
}

func extractLatency(line string) string {
	index := strings.LastIndex(line, " in ")
	if index == -1 {
		return ""
	}
	latency := strings.TrimSpace(line[index+len(" in "):])
	if fields := strings.Fields(latency); len(fields) > 0 {
		return fields[0]
	}
	return latency
}
