package firewall

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

const defaultRulePrefix = "portshare"

type TrustedPeerAccess struct {
	RulePrefix       string
	LocalTailscaleIP string
	PeerTailscaleIP  string
	PeerID           string
	PeerName         string
}

type Rule struct {
	Name      string
	Direction string
	Action    string
	Protocol  string
	LocalIP   string
	RemoteIP  string
	LocalPort string
}

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type Authorizer struct {
	runner CommandRunner
}

func NewAuthorizer(runner CommandRunner) *Authorizer {
	if runner == nil {
		runner = newDefaultRunner()
	}
	return &Authorizer{runner: runner}
}

func (a *Authorizer) AllowTrustedPeer(ctx context.Context, access TrustedPeerAccess) error {
	if a == nil {
		return errors.New("firewall authorizer is not configured")
	}
	rules, err := BuildTrustedPeerRules(access)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		_, _ = a.runner.Run(ctx, "netsh", deleteRuleArgs(rule)...)
		output, err := a.runner.Run(ctx, "netsh", addRuleArgs(rule)...)
		if err != nil {
			return describeRuleError(rule, output, err)
		}
	}
	return nil
}

func (a *Authorizer) RevokeTrustedPeer(ctx context.Context, access TrustedPeerAccess) error {
	if a == nil {
		return errors.New("firewall authorizer is not configured")
	}
	rules, err := BuildTrustedPeerRules(access)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		output, err := a.runner.Run(ctx, "netsh", deleteRuleArgs(rule)...)
		if err != nil {
			if isFirewallPermissionError(output, err) {
				return describeDeleteRuleError(rule, output, err)
			}
			if isDeleteRuleNotFoundError(output, err) {
				continue
			}
			return describeDeleteRuleError(rule, output, err)
		}
	}
	return nil
}

func BuildTrustedPeerRules(access TrustedPeerAccess) ([]Rule, error) {
	localIP := strings.TrimSpace(access.LocalTailscaleIP)
	peerIP := strings.TrimSpace(access.PeerTailscaleIP)
	if net.ParseIP(localIP) == nil {
		return nil, fmt.Errorf("本机 Tailscale IP 无效或缺失：%q", access.LocalTailscaleIP)
	}
	if net.ParseIP(peerIP) == nil {
		return nil, fmt.Errorf("对方 Tailscale IP 无效或缺失：%q", access.PeerTailscaleIP)
	}

	prefix := strings.TrimSpace(access.RulePrefix)
	if prefix == "" {
		prefix = defaultRulePrefix
	}
	peerLabel := strings.TrimSpace(access.PeerID)
	if peerLabel == "" {
		peerLabel = strings.TrimSpace(access.PeerName)
	}
	if peerLabel == "" {
		peerLabel = peerIP
	}

	rules := make([]Rule, 0, 2)
	for _, protocol := range []string{"TCP", "UDP"} {
		rules = append(rules, Rule{
			Name:      makeRuleName(prefix, peerLabel, peerIP, protocol),
			Direction: "in",
			Action:    "allow",
			Protocol:  protocol,
			LocalIP:   localIP,
			RemoteIP:  peerIP,
			LocalPort: "any",
		})
	}
	return rules, nil
}

func makeRuleName(prefix, peerLabel, peerIP, protocol string) string {
	parts := []string{
		sanitizeRulePart(prefix),
		"trusted",
		sanitizeRulePart(peerLabel),
		sanitizeRulePart(peerIP),
		strings.ToLower(protocol),
	}
	return strings.Join(parts, "-")
}

func sanitizeRulePart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if allowed {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "device"
	}
	return result
}

func deleteRuleArgs(rule Rule) []string {
	return []string{
		"advfirewall",
		"firewall",
		"delete",
		"rule",
		"name=" + rule.Name,
	}
}

func addRuleArgs(rule Rule) []string {
	return []string{
		"advfirewall",
		"firewall",
		"add",
		"rule",
		"name=" + rule.Name,
		"dir=" + rule.Direction,
		"action=" + rule.Action,
		"protocol=" + rule.Protocol,
		"localip=" + rule.LocalIP,
		"remoteip=" + rule.RemoteIP,
		"localport=" + rule.LocalPort,
		"profile=any",
		"enable=yes",
	}
}

func describeDeleteRuleError(rule Rule, output []byte, err error) error {
	details := strings.TrimSpace(string(output))
	text := strings.ToLower(details + " " + err.Error())
	if strings.Contains(text, "elevat") || strings.Contains(text, "administrator") || strings.Contains(text, "access is denied") || strings.Contains(text, "拒绝访问") {
		return fmt.Errorf("删除 Windows 防火墙规则 %q 失败：请以管理员身份运行 portshare 后重试：%w", rule.Name, err)
	}
	if details == "" {
		return fmt.Errorf("删除 Windows 防火墙规则 %q 失败：%w", rule.Name, err)
	}
	return fmt.Errorf("删除 Windows 防火墙规则 %q 失败：%s：%w", rule.Name, details, err)
}

func isDeleteRuleNotFoundError(output []byte, err error) bool {
	if err == nil {
		return false
	}
	details := strings.TrimSpace(string(output))
	text := strings.ToLower(details + " " + err.Error())
	notFoundMarkers := []string{
		"no rules match",
		"no rule matches",
		"no matching rule",
		"no matching rules",
		"rule was not found",
		"rule not found",
		"does not exist",
		"not exist",
		"not found",
		"没有规则",
		"未找到",
		"找不到",
	}
	for _, marker := range notFoundMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isFirewallPermissionError(output []byte, err error) bool {
	if err == nil {
		return false
	}
	details := strings.TrimSpace(string(output))
	text := strings.ToLower(details + " " + err.Error())
	return strings.Contains(text, "elevat") ||
		strings.Contains(text, "administrator") ||
		strings.Contains(text, "access is denied") ||
		strings.Contains(text, "鎷掔粷璁块棶")
}

func describeRuleError(rule Rule, output []byte, err error) error {
	details := strings.TrimSpace(string(output))
	text := strings.ToLower(details + " " + err.Error())
	if strings.Contains(text, "elevat") || strings.Contains(text, "administrator") || strings.Contains(text, "access is denied") || strings.Contains(text, "拒绝访问") {
		return fmt.Errorf("写入 Windows 防火墙规则 %q 失败：请以管理员身份运行 portshare 后重试：%w", rule.Name, err)
	}
	if details == "" {
		return fmt.Errorf("写入 Windows 防火墙规则 %q 失败：%w", rule.Name, err)
	}
	return fmt.Errorf("写入 Windows 防火墙规则 %q 失败：%s：%w", rule.Name, details, err)
}
