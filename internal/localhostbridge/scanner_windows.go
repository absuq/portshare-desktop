//go:build windows

package localhostbridge

import (
	"context"
	"os/exec"

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
	return parsePowerShellTCPListeners(output)
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
