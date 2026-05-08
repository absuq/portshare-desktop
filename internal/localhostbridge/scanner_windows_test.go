//go:build windows

package localhostbridge

import (
	"context"
	"testing"

	"github.com/absuq/portshare-desktop/internal/winexec"
)

func TestNewPowerShellScanCommandHidesWindow(t *testing.T) {
	cmd := newPowerShellScanCommand(context.Background())
	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected PowerShell scan command to hide its window")
	}
	if cmd.SysProcAttr.CreationFlags&winexec.CreateNoWindow == 0 {
		t.Fatalf("expected CREATE_NO_WINDOW creation flag, got %#x", cmd.SysProcAttr.CreationFlags)
	}
}
