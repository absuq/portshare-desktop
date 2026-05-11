package netdiag

import "time"

type PathStatus string

const (
	PathUnknown            PathStatus = "unknown"
	PathDirectNormal       PathStatus = "direct-normal"
	PathDirectProxy        PathStatus = "direct-proxy"
	PathDirectTUNOptimized PathStatus = "direct-tun-optimized"
	PathDERP               PathStatus = "derp"
	PathFailed             PathStatus = "failed"
)

const (
	RouteUnknown   = "unknown"
	RouteDirect    = "direct"
	RouteDERP      = "derp"
	RoutePeerRelay = "peer-relay"
)

const (
	AddressFamilyIPv4 = "IPv4"
	AddressFamilyIPv6 = "IPv6"
)

type PeerPathReport struct {
	PeerTailscaleIP string
	Status          PathStatus
	RouteType       string
	Endpoint        string
	EndpointIP      string
	Latency         string
	CurrentRoute    RouteInfo
	Candidates      []EgressCandidate
	Message         string
}

type RouteInfo struct {
	InterfaceAlias string
	InterfaceIndex int
	NextHop        string
	IPAddress      string
	AddressFamily  string
}

type EgressCandidate struct {
	InterfaceAlias  string
	InterfaceIndex  int
	InterfaceIP     string
	NextHop         string
	AddressFamily   string
	PublicIPv4      string
	PublicIPv6      string
	UDP             bool
	NetcheckError   string
	RouteMetric     int
	InterfaceMetric int
	SuspectedProxy  bool
	Recommended     bool
}

type BypassRequest struct {
	PeerTailscaleIP string
	EndpointIP      string
	Candidate       EgressCandidate
}

type ActiveBypass struct {
	PeerTailscaleIP string
	EndpointIP      string
	AddressFamily   string
	InterfaceIndex  int
	NextHop         string
	CreatedAt       time.Time
}
