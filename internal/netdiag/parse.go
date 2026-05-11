package netdiag

import (
	"net"
	"strings"
	"time"
)

const optimizedTUNDirectLatency = 50 * time.Millisecond

func ParsePingRoute(raw []byte) (routeType, endpoint, latency string) {
	var relayType, relayEndpoint, relayLatency string
	for _, line := range strings.Split(string(raw), "\n") {
		candidateType, candidateEndpoint, candidateLatency, ok := parsePingLine(line)
		if !ok {
			continue
		}
		if candidateType == RouteDirect {
			return candidateType, candidateEndpoint, candidateLatency
		}
		if relayType == "" {
			relayType, relayEndpoint, relayLatency = candidateType, candidateEndpoint, candidateLatency
		}
	}
	if relayType != "" {
		return relayType, relayEndpoint, relayLatency
	}
	return RouteUnknown, "", ""
}

func EndpointIP(endpoint string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String()
	}
	return ip.String()
}

func IsPublicIPv4(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || ip.To4() == nil {
		return false
	}
	ip = ip.To4()
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return false
	}
	first, second := ip[0], ip[1]
	if first == 100 && second >= 64 && second <= 127 {
		return false
	}
	if first == 198 && (second == 18 || second == 19) {
		return false
	}
	return true
}

func IsPublicEndpointIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return false
	}
	if ip.To4() != nil {
		return IsPublicIPv4(value)
	}
	return isPublicIPv6(ip)
}

func endpointAddressFamily(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	if ip.To4() != nil {
		return AddressFamilyIPv4
	}
	return AddressFamilyIPv6
}

func isPublicIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return false
	}
	return true
}

func IsSuspectedProxyInterface(alias string) bool {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" {
		return false
	}
	for _, marker := range []string{"meta", "clash", "mihomo", "vortex", "tun", "proxy", "sing-box", "nekoray"} {
		if strings.Contains(alias, marker) {
			return true
		}
	}
	return false
}

func ClassifyPath(routeType string, latency string, current RouteInfo) PathStatus {
	switch routeType {
	case RouteDirect:
		if IsSuspectedProxyInterface(current.InterfaceAlias) {
			if isOptimizedDirectLatency(latency) {
				return PathDirectTUNOptimized
			}
			return PathDirectProxy
		}
		return PathDirectNormal
	case RouteDERP, RoutePeerRelay:
		return PathDERP
	default:
		return PathUnknown
	}
}

func isOptimizedDirectLatency(latency string) bool {
	duration, err := time.ParseDuration(strings.TrimSpace(latency))
	if err != nil || duration <= 0 {
		return false
	}
	return duration <= optimizedTUNDirectLatency
}

func parsePingLine(line string) (routeType, endpoint, latency string, ok bool) {
	line = strings.TrimSpace(line)
	const viaMarker = " via "
	viaIndex := strings.Index(line, viaMarker)
	if viaIndex == -1 {
		return "", "", "", false
	}
	rest := line[viaIndex+len(viaMarker):]
	inIndex := strings.Index(rest, " in ")
	if inIndex == -1 {
		return "", "", "", false
	}
	endpoint = strings.TrimSpace(rest[:inIndex])
	fields := strings.Fields(strings.TrimSpace(rest[inIndex+len(" in "):]))
	if len(fields) == 0 {
		return "", "", "", false
	}
	latency = fields[0]
	switch {
	case strings.HasPrefix(endpoint, "DERP("):
		return RouteDERP, endpoint, latency, true
	case strings.HasPrefix(endpoint, "peer-relay("):
		return RoutePeerRelay, endpoint, latency, true
	case EndpointIP(endpoint) != "":
		return RouteDirect, endpoint, latency, true
	default:
		return RouteUnknown, endpoint, latency, true
	}
}
