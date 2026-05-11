package netdiag

import "testing"

func TestParsePingRouteFindsDirectEndpoint(t *testing.T) {
	routeType, endpoint, latency := ParsePingRoute([]byte("pong from desktop via 115.233.222.82:41641 in 15ms"))

	if routeType != RouteDirect || endpoint != "115.233.222.82:41641" || latency != "15ms" {
		t.Fatalf("unexpected route: type=%q endpoint=%q latency=%q", routeType, endpoint, latency)
	}
}

func TestParsePingRoutePrefersDirectAfterDERP(t *testing.T) {
	raw := []byte("pong from desktop via DERP(tok) in 88ms\npong from desktop via 115.233.222.82:41641 in 15ms")

	routeType, endpoint, latency := ParsePingRoute(raw)

	if routeType != RouteDirect || endpoint != "115.233.222.82:41641" || latency != "15ms" {
		t.Fatalf("unexpected route: type=%q endpoint=%q latency=%q", routeType, endpoint, latency)
	}
}

func TestParsePingRouteFindsDERP(t *testing.T) {
	routeType, endpoint, latency := ParsePingRoute([]byte("pong from desktop via DERP(hkg) in 43ms"))

	if routeType != RouteDERP || endpoint != "DERP(hkg)" || latency != "43ms" {
		t.Fatalf("unexpected route: type=%q endpoint=%q latency=%q", routeType, endpoint, latency)
	}
}

func TestEndpointIPExtractsIPv4(t *testing.T) {
	if got := EndpointIP("115.233.222.82:41641"); got != "115.233.222.82" {
		t.Fatalf("EndpointIP() = %q", got)
	}
}

func TestIsPublicIPv4RejectsPrivateAndProxyRanges(t *testing.T) {
	public := []string{"115.233.222.82", "8.8.8.8"}
	for _, ip := range public {
		if !IsPublicIPv4(ip) {
			t.Fatalf("expected %s to be public", ip)
		}
	}

	nonPublic := []string{"", "127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "100.64.0.1", "198.18.0.1", "224.0.0.1", "::1"}
	for _, ip := range nonPublic {
		if IsPublicIPv4(ip) {
			t.Fatalf("expected %s to be non-public", ip)
		}
	}
}

func TestIsSuspectedProxyInterface(t *testing.T) {
	for _, name := range []string{"Meta", "Clash Tunnel", "mihomo", "Vortex TUN"} {
		if !IsSuspectedProxyInterface(name) {
			t.Fatalf("expected %q to be suspected proxy", name)
		}
	}
	if IsSuspectedProxyInterface("以太网") {
		t.Fatal("expected physical ethernet name to be non-proxy")
	}
}

func TestClassifyPathDetectsDirectProxy(t *testing.T) {
	status := ClassifyPath(RouteDirect, "249ms", RouteInfo{InterfaceAlias: "Meta"})
	if status != PathDirectProxy {
		t.Fatalf("expected proxy path, got %s", status)
	}
}

func TestClassifyPathDetectsProxyInterfaceEvenWhenFast(t *testing.T) {
	status := ClassifyPath(RouteDirect, "11ms", RouteInfo{InterfaceAlias: "Meta"})
	if status != PathDirectProxy {
		t.Fatalf("expected proxy path, got %s", status)
	}
}

func TestClassifyPathDetectsDirectNormal(t *testing.T) {
	status := ClassifyPath(RouteDirect, "11ms", RouteInfo{InterfaceAlias: "以太网"})
	if status != PathDirectNormal {
		t.Fatalf("expected normal direct path, got %s", status)
	}
}

func TestParsePingRouteIgnoresMalformedLatency(t *testing.T) {
	routeType, endpoint, latency := ParsePingRoute([]byte("pong from desktop via 115.233.222.82:41641 in "))
	if routeType != RouteUnknown || endpoint != "" || latency != "" {
		t.Fatalf("expected malformed line to be ignored, got type=%q endpoint=%q latency=%q", routeType, endpoint, latency)
	}
}
