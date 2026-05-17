package clash

import "time"

type ClashConfig struct {
	MixedPort              int
	SocksPort              int
	HTTPPort               int
	ExternalController     string
	ExternalControllerPipe string
	Secret                 string
	AllowLAN               bool
	TUNEnabled             bool
	SourcePath             string
}

type ProxyPort struct {
	Kind string
	Port int
}

type ControlKind string

const (
	ControlNone      ControlKind = ""
	ControlHTTP      ControlKind = "http"
	ControlNamedPipe ControlKind = "named-pipe"
)

type ControlEndpoint struct {
	Kind    ControlKind
	Address string
	Secret  string
}

type TUNInterface struct {
	Name        string
	Description string
	Index       int
	Status      string
}

type DiscoveryReport struct {
	Config        ClashConfig
	TUNInterfaces []TUNInterface
	ProxyPorts    []ProxyPort
	Control       ControlEndpoint
	Nodes         []ProxyNode
	Message       string
}

type ProxyNode struct {
	GroupName        string
	Name             string
	Type             string
	Region           string
	Current          bool
	Delay            time.Duration
	TailscaleRoute   string
	TailscaleLatency string
	Recommended      bool
}

type ApplyRequest struct {
	PeerTailscaleIP string
	GroupName       string
	NodeName        string
	PreviousNode    string
}

type ApplyResult struct {
	GroupName        string
	NodeName         string
	PreviousNode     string
	RouteType        string
	Endpoint         string
	Latency          string
	Improved         bool
	RestoredPrevious bool
}

type RouteCheck struct {
	RouteType string
	Endpoint  string
	Latency   string
}

type Version struct {
	Version string
}

type ProxySnapshot struct {
	Groups []ProxyGroup
}

type ProxyGroup struct {
	Name    string
	Type    string
	Now     string
	Options []ProxyOption
}

type ProxyOption struct {
	Name  string
	Type  string
	Delay time.Duration
}
