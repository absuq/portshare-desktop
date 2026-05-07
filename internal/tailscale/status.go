package tailscale

import (
	"encoding/json"
	"net"
	"strings"
)

type Status struct {
	BackendState    string
	LocalIPv4       string
	SelfHostName    string
	SelfDNSName     string
	MagicDNSEnabled bool
	MagicDNSSuffix  string
}

func ParseStatus(raw []byte) (Status, error) {
	var payload struct {
		BackendState   string   `json:"BackendState"`
		TailscaleIPs   []string `json:"TailscaleIPs"`
		Self           selfNode `json:"Self"`
		CurrentTailnet struct {
			MagicDNSEnabled bool   `json:"MagicDNSEnabled"`
			MagicDNSSuffix  string `json:"MagicDNSSuffix"`
		} `json:"CurrentTailnet"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Status{}, err
	}

	return Status{
		BackendState:    payload.BackendState,
		LocalIPv4:       firstIPv4(payload.TailscaleIPs),
		SelfHostName:    payload.Self.HostName,
		SelfDNSName:     strings.TrimSuffix(payload.Self.DNSName, "."),
		MagicDNSEnabled: payload.CurrentTailnet.MagicDNSEnabled,
		MagicDNSSuffix:  payload.CurrentTailnet.MagicDNSSuffix,
	}, nil
}

type selfNode struct {
	HostName string `json:"HostName"`
	DNSName  string `json:"DNSName"`
}

func firstIPv4(ips []string) string {
	for _, ipText := range ips {
		ip := net.ParseIP(ipText)
		if ip == nil {
			continue
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String()
		}
	}
	return ""
}
