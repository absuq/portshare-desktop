//go:build windows

package winexec

import (
	"context"
	"testing"
)

func TestNewCommandHidesChildProcessWindow(t *testing.T) {
	cmd := NewCommand(context.Background(), "powershell.exe", "-NoProfile")
	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected child process window to be hidden")
	}
	if cmd.SysProcAttr.CreationFlags&CreateNoWindow == 0 {
		t.Fatalf("expected CREATE_NO_WINDOW flag, got %#x", cmd.SysProcAttr.CreationFlags)
	}
}
