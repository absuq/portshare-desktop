//go:build windows

package localhostbridge

import (
	"context"
	"os/exec"
)

type WindowsScanner struct{}

func NewScanner() Scanner {
	return WindowsScanner{}
}

func (WindowsScanner) Scan(ctx context.Context) ([]ListeningPort, error) {
	output, err := exec.CommandContext(
		ctx,
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"Get-NetTCPConnection -State Listen | Select-Object LocalAddress,LocalPort | ConvertTo-Json -Compress",
	).Output()
	if err != nil {
		return nil, err
	}
	return parsePowerShellTCPListeners(output)
}
