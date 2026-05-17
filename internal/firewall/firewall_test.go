package firewall

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	commands []recordedCommand
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	return []byte("ok"), nil
}

type scriptedRunner struct {
	commands []recordedCommand
	results  []scriptedResult
}

type scriptedResult struct {
	output []byte
	err    error
}

func (r *scriptedRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	if len(r.results) == 0 {
		return nil, errors.New("unexpected command")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result.output, result.err
}

func TestBuildTrustedPeerRulesCreatesTCPAndUDPScopedToTailnetIPs(t *testing.T) {
	rules, err := BuildTrustedPeerRules(TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "desktop-bgpql0r",
		PeerName:         "DESKTOP-BGPQL0R",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected TCP and UDP rules, got %+v", rules)
	}

	for _, rule := range rules {
		if !strings.Contains(rule.Name, "portshare") || !strings.Contains(rule.Name, "desktop-bgpql0r") {
			t.Fatalf("expected stable portshare rule name, got %q", rule.Name)
		}
		if rule.Direction != "in" || rule.Action != "allow" {
			t.Fatalf("expected inbound allow rule, got %+v", rule)
		}
		if rule.LocalIP != "100.79.83.104" || rule.RemoteIP != "100.109.251.97" {
			t.Fatalf("expected rule to be scoped to tailscale IPs, got %+v", rule)
		}
		if rule.LocalPort != "any" {
			t.Fatalf("expected all local ports, got %+v", rule)
		}
	}
	if rules[0].Protocol != "TCP" || rules[1].Protocol != "UDP" {
		t.Fatalf("expected TCP then UDP rules, got %+v", rules)
	}
}

func TestAuthorizerReplacesTCPAndUDPRules(t *testing.T) {
	runner := &recordingRunner{}
	authorizer := NewAuthorizer(runner)

	err := authorizer.AllowTrustedPeer(context.Background(), TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "desktop-bgpql0r",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.commands) != 4 {
		t.Fatalf("expected delete/add for TCP and UDP, got %+v", runner.commands)
	}
	if runner.commands[0].name != "netsh" || runner.commands[1].name != "netsh" {
		t.Fatalf("expected netsh commands, got %+v", runner.commands[:2])
	}
	if !containsArg(runner.commands[0].args, "delete") || !containsArgPrefix(runner.commands[0].args, "name=portshare") {
		t.Fatalf("expected first command to delete old TCP rule, got %+v", runner.commands[0])
	}
	if !containsArg(runner.commands[1].args, "add") ||
		!containsArg(runner.commands[1].args, "protocol=TCP") ||
		!containsArg(runner.commands[1].args, "localip=100.79.83.104") ||
		!containsArg(runner.commands[1].args, "remoteip=100.109.251.97") ||
		!containsArg(runner.commands[1].args, "localport=any") {
		t.Fatalf("expected second command to add scoped TCP rule, got %+v", runner.commands[1])
	}
	if !containsArg(runner.commands[3].args, "protocol=UDP") {
		t.Fatalf("expected fourth command to add UDP rule, got %+v", runner.commands[3])
	}
}

func TestAuthorizerRevokesTCPAndUDPRules(t *testing.T) {
	runner := &recordingRunner{}
	authorizer := NewAuthorizer(runner)

	err := authorizer.RevokeTrustedPeer(context.Background(), TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "desktop-bgpql0r",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("expected delete commands for TCP and UDP, got %+v", runner.commands)
	}
	for _, command := range runner.commands {
		if command.name != "netsh" {
			t.Fatalf("expected netsh command, got %+v", command)
		}
		if !containsArg(command.args, "delete") || !containsArgPrefix(command.args, "name=portshare") {
			t.Fatalf("expected delete rule command, got %+v", command)
		}
		if containsArg(command.args, "add") {
			t.Fatalf("revoke must not add firewall rules, got %+v", command)
		}
	}
}

func TestAuthorizerRevokeContinuesWhenRuleDoesNotExist(t *testing.T) {
	runner := &scriptedRunner{
		results: []scriptedResult{
			{output: []byte("No rules match the specified criteria."), err: errors.New("exit status 1")},
			{output: []byte("Deleted 1 rule(s)."), err: nil},
		},
	}
	authorizer := NewAuthorizer(runner)

	err := authorizer.RevokeTrustedPeer(context.Background(), TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "desktop-bgpql0r",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("expected both delete commands to run, got %+v", runner.commands)
	}
	for _, command := range runner.commands {
		if !containsArg(command.args, "delete") || !containsArgPrefix(command.args, "name=portshare") {
			t.Fatalf("expected delete rule command, got %+v", command)
		}
	}
}

func TestAuthorizerRevokeStillFailsPermissionErrors(t *testing.T) {
	runner := &scriptedRunner{
		results: []scriptedResult{
			{output: []byte("Access is denied. No rules match the specified criteria."), err: errors.New("exit status 1")},
		},
	}
	authorizer := NewAuthorizer(runner)

	err := authorizer.RevokeTrustedPeer(context.Background(), TrustedPeerAccess{
		RulePrefix:       "portshare",
		LocalTailscaleIP: "100.79.83.104",
		PeerTailscaleIP:  "100.109.251.97",
		PeerID:           "desktop-bgpql0r",
	})
	if err == nil {
		t.Fatal("expected permission error to fail revoke")
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected revoke to stop on permission error, got %+v", runner.commands)
	}
}

func TestBuildTrustedPeerRulesRejectsMissingIPs(t *testing.T) {
	if _, err := BuildTrustedPeerRules(TrustedPeerAccess{LocalTailscaleIP: "", PeerTailscaleIP: "100.109.251.97"}); err == nil {
		t.Fatal("expected missing local tailscale IP to fail")
	}
	if _, err := BuildTrustedPeerRules(TrustedPeerAccess{LocalTailscaleIP: "100.79.83.104", PeerTailscaleIP: ""}); err == nil {
		t.Fatal("expected missing peer tailscale IP to fail")
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}
