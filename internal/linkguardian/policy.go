package linkguardian

import (
	"fmt"
	"strings"
	"time"

	"github.com/absuq/portshare-desktop/internal/netdiag"
)

func Evaluate(input EvaluateInput) Decision {
	path := input.Path
	if input.HasActiveBypass && shouldClearActiveBypass(path, input.ActiveBypass) {
		return Decision{
			Status:  StatusRollback,
			Action:  ActionClearBypass,
			Message: withLatencySample("endpoint 已变化，准备撤销旧的精确绕过", input.LatestLatency),
		}
	}

	switch path.Status {
	case netdiag.PathDirectNormal:
		return Decision{
			Status:  StatusOptimized,
			Action:  ActionWatch,
			Message: withLatencySample("当前已经是低延迟直连", input.LatestLatency),
		}
	case netdiag.PathDirectTUNOptimized:
		return Decision{
			Status:  StatusOptimized,
			Action:  ActionWatch,
			Message: withLatencySample("TUN 接管但当前是低延迟直连", input.LatestLatency),
		}
	case netdiag.PathDirectProxy:
		candidate, ok := RecommendedCandidate(path.Candidates)
		if input.AutoBypass && strings.TrimSpace(path.EndpointIP) != "" && ok {
			return Decision{
				Status:    StatusBypassReady,
				Action:    ActionApplyBypass,
				Candidate: candidate,
				Message:   withLatencySample("当前 direct 高延迟且疑似 TUN 绕路，准备应用 endpoint 精确绕过", input.LatestLatency),
			}
		}
		return Decision{
			Status:  StatusBypassReady,
			Action:  ActionWatch,
			Message: withLatencySample("当前 direct 高延迟且疑似 TUN 绕路", input.LatestLatency),
		}
	case netdiag.PathDERP:
		return Decision{
			Status:  StatusRelay,
			Action:  ActionReprobe,
			Message: withLatencySample("当前仍在中继，先重新探测 Tailscale 直连", input.LatestLatency),
		}
	case netdiag.PathFailed:
		return Decision{
			Status:  StatusFailed,
			Action:  ActionWatch,
			Message: withLatencySample("链路检测失败", input.LatestLatency),
		}
	default:
		return Decision{
			Status:  StatusIdle,
			Action:  ActionWatch,
			Message: withLatencySample("链路状态未知", input.LatestLatency),
		}
	}
}

func RecommendedCandidate(candidates []netdiag.EgressCandidate) (netdiag.EgressCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.Recommended {
			return candidate, true
		}
	}
	for _, candidate := range candidates {
		if candidate.InterfaceIndex > 0 && strings.TrimSpace(candidate.NextHop) != "" && !candidate.SuspectedProxy {
			return candidate, true
		}
	}
	return netdiag.EgressCandidate{}, false
}

func shouldClearActiveBypass(path netdiag.PeerPathReport, active netdiag.ActiveBypass) bool {
	endpoint := strings.TrimSpace(path.EndpointIP)
	activeEndpoint := strings.TrimSpace(active.EndpointIP)
	if endpoint == "" || activeEndpoint == "" {
		return false
	}
	return endpoint != activeEndpoint
}

func withLatencySample(message string, latency time.Duration) string {
	if latency <= 0 {
		return message
	}
	return fmt.Sprintf("%s（主页延迟 %dms）", message, latency.Milliseconds())
}
