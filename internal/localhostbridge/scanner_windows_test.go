//go:build windows

package localhostbridge

import (
	"context"
	"reflect"
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

func TestParseWSLDistributionNamesHandlesNULPaddedOutput(t *testing.T) {
	input := []byte("U\x00b\x00u\x00n\x00t\x00u\x00-\x002\x004\x00.\x000\x004\x00\r\x00\n\x00d\x00o\x00c\x00k\x00e\x00r\x00-\x00d\x00e\x00s\x00k\x00t\x00o\x00p\x00\r\x00\n\x00")

	got := parseWSLDistributionNames(input)

	want := []string{"Ubuntu-24.04", "docker-desktop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWSLDistributionNames() = %#v, want %#v", got, want)
	}
}

func TestParseWSLSSListenersIncludesLoopbackOnly(t *testing.T) {
	input := []byte(`LISTEN 0      1000   10.255.255.254:53    0.0.0.0:*
LISTEN 0      4096        127.0.0.1:18789 0.0.0.0:*
LISTEN 0      4096       127.0.0.54:53    0.0.0.0:*
LISTEN 0      5              [::1]:3000        [::]:*
LISTEN 0      4096                *:8317        *:*
`)

	got := parseWSLSSListeners(input)

	want := []ListeningPort{
		{Address: "127.0.0.1", Port: 18789},
		{Address: "::1", Port: 3000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseWSLSSListeners() = %#v, want %#v", got, want)
	}
}

func TestMergeListeningPortsDeduplicatesWindowsAndWSL(t *testing.T) {
	windowsListeners := []ListeningPort{
		{Address: "127.0.0.1", Port: 18789},
		{Address: "0.0.0.0", Port: 8317},
	}
	wslListeners := []ListeningPort{
		{Address: "127.0.0.1", Port: 18789},
		{Address: "127.0.0.1", Port: 3000},
	}

	got := mergeListeningPorts(windowsListeners, wslListeners)

	want := []ListeningPort{
		{Address: "127.0.0.1", Port: 18789},
		{Address: "0.0.0.0", Port: 8317},
		{Address: "127.0.0.1", Port: 3000},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeListeningPorts() = %#v, want %#v", got, want)
	}
}

func TestNewWSLCommandsHideWindow(t *testing.T) {
	listCmd := newWSLListCommand(context.Background())
	if listCmd.SysProcAttr == nil || !listCmd.SysProcAttr.HideWindow {
		t.Fatal("expected WSL list command to hide its window")
	}

	ssCmd := newWSLSSCommand(context.Background(), "Ubuntu-24.04")
	if ssCmd.SysProcAttr == nil || !ssCmd.SysProcAttr.HideWindow {
		t.Fatal("expected WSL ss command to hide its window")
	}
}
