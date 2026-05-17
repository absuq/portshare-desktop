package localhostbridge

import (
	"bytes"
	"context"
	"encoding/json"
)

type powerShellTCPListener struct {
	LocalAddress string `json:"LocalAddress"`
	LocalPort    int    `json:"LocalPort"`
}

func parsePowerShellTCPListeners(data []byte) ([]ListeningPort, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '[' {
		var raw []powerShellTCPListener
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		return listeningPortsFromRaw(raw), nil
	}
	var single powerShellTCPListener
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, err
	}
	return listeningPortsFromRaw([]powerShellTCPListener{single}), nil
}

func listeningPortsFromRaw(raw []powerShellTCPListener) []ListeningPort {
	listeners := make([]ListeningPort, 0, len(raw))
	for _, item := range raw {
		if item.LocalAddress == "" || item.LocalPort <= 0 || item.LocalPort > 65535 {
			continue
		}
		listeners = append(listeners, ListeningPort{Address: item.LocalAddress, Port: item.LocalPort})
	}
	return listeners
}

type emptyScanner struct{}

func (emptyScanner) Scan(context.Context) ([]ListeningPort, error) {
	return nil, nil
}
