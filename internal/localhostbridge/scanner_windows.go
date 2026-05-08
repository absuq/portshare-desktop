//go:build windows

package localhostbridge

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/absuq/portshare-desktop/internal/winexec"
)

type WindowsScanner struct{}

func NewScanner() Scanner {
	return WindowsScanner{}
}

func (WindowsScanner) Scan(ctx context.Context) ([]ListeningPort, error) {
	cmd := newPowerShellScanCommand(ctx)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	windowsListeners, err := parsePowerShellTCPListeners(output)
	if err != nil {
		return nil, err
	}
	wslListeners := scanWSLListeners(ctx)
	return mergeListeningPorts(windowsListeners, wslListeners), nil
}

func newPowerShellScanCommand(ctx context.Context) *exec.Cmd {
	return winexec.NewCommand(
		ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"Get-NetTCPConnection -State Listen | Select-Object LocalAddress,LocalPort | ConvertTo-Json -Compress",
	)
}

func scanWSLListeners(ctx context.Context) []ListeningPort {
	output, err := newWSLListCommand(ctx).Output()
	if err != nil {
		return nil
	}
	distros := parseWSLDistributionNames(output)
	var listeners []ListeningPort
	for _, distro := range distros {
		output, err := newWSLSSCommand(ctx, distro).Output()
		if err != nil {
			continue
		}
		listeners = append(listeners, parseWSLSSListeners(output)...)
	}
	return listeners
}

func newWSLListCommand(ctx context.Context) *exec.Cmd {
	return winexec.NewCommand(ctx, "wsl.exe", "--list", "--quiet", "--running")
}

func newWSLSSCommand(ctx context.Context, distro string) *exec.Cmd {
	return winexec.NewCommand(ctx, "wsl.exe", "--distribution", distro, "--exec", "sh", "-lc", "ss -ltnH")
}

func parseWSLDistributionNames(data []byte) []string {
	data = bytes.ReplaceAll(data, []byte{0}, nil)
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\r' || r == '\n'
	})
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func parseWSLSSListeners(data []byte) []ListeningPort {
	lines := strings.Split(string(data), "\n")
	listeners := make([]ListeningPort, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "LISTEN" {
			continue
		}
		host, port, ok := splitSSLocalAddress(fields[3])
		if !ok || !isForwardedWSLLoopback(host) {
			continue
		}
		listeners = append(listeners, ListeningPort{Address: host, Port: port})
	}
	return listeners
}

func splitSSLocalAddress(value string) (string, int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", 0, false
	}
	if strings.HasPrefix(value, "[") {
		end := strings.LastIndex(value, "]:")
		if end <= 0 {
			return "", 0, false
		}
		host := value[1:end]
		port, ok := parsePort(value[end+2:])
		return stripZone(host), port, ok
	}
	index := strings.LastIndex(value, ":")
	if index <= 0 || index == len(value)-1 {
		return "", 0, false
	}
	port, ok := parsePort(value[index+1:])
	return stripZone(value[:index]), port, ok
}

func parsePort(value string) (int, bool) {
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func stripZone(host string) string {
	if index := strings.LastIndex(host, "%"); index >= 0 {
		return host[:index]
	}
	return host
}

func isForwardedWSLLoopback(host string) bool {
	return host == "127.0.0.1" || host == "::1"
}

func mergeListeningPorts(groups ...[]ListeningPort) []ListeningPort {
	seen := map[ListeningPort]struct{}{}
	var merged []ListeningPort
	for _, group := range groups {
		for _, listener := range group {
			if _, ok := seen[listener]; ok {
				continue
			}
			seen[listener] = struct{}{}
			merged = append(merged, listener)
		}
	}
	return merged
}
