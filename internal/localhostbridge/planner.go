package localhostbridge

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

type ListeningPort struct {
	Address string
	Port    int
}

type PlanInput struct {
	LocalTailscaleIP string
	AllowedPeerIPs   []string
	Listeners        []ListeningPort
}

type BridgePlan struct {
	Port           int
	ListenAddress  string
	TargetAddress  string
	AllowedPeerIPs []string
}

func BuildPlan(input PlanInput) []BridgePlan {
	localIP := strings.TrimSpace(input.LocalTailscaleIP)
	allowed := normalizeAllowedPeerIPs(input.AllowedPeerIPs)
	if localIP == "" || len(allowed) == 0 {
		return nil
	}

	byPort := map[int][]string{}
	for _, listener := range input.Listeners {
		if listener.Port <= 0 || listener.Port > 65535 {
			continue
		}
		address := strings.TrimSpace(listener.Address)
		if address == "" {
			continue
		}
		byPort[listener.Port] = append(byPort[listener.Port], address)
	}

	ports := make([]int, 0, len(byPort))
	for port := range byPort {
		ports = append(ports, port)
	}
	sort.Ints(ports)

	var plans []BridgePlan
	for _, port := range ports {
		addresses := byPort[port]
		if !hasLoopbackListener(addresses) || hasNativeReachableListener(addresses, localIP) {
			continue
		}
		plans = append(plans, BridgePlan{
			Port:           port,
			ListenAddress:  net.JoinHostPort(localIP, fmt.Sprintf("%d", port)),
			TargetAddress:  net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)),
			AllowedPeerIPs: append([]string(nil), allowed...),
		})
	}
	return plans
}

func normalizeAllowedPeerIPs(values []string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func hasLoopbackListener(addresses []string) bool {
	for _, address := range addresses {
		if isLoopbackAddress(address) {
			return true
		}
	}
	return false
}

func hasNativeReachableListener(addresses []string, localTailscaleIP string) bool {
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "0.0.0.0" || address == "::" || address == localTailscaleIP {
			return true
		}
	}
	return false
}

func isLoopbackAddress(address string) bool {
	ip := net.ParseIP(strings.Trim(address, "[]"))
	return ip != nil && ip.IsLoopback()
}
