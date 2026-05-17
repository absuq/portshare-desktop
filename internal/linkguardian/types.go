package linkguardian

import (
	"time"

	"github.com/absuq/portshare-desktop/internal/netdiag"
)

type Status string

const (
	StatusIdle          Status = "idle"
	StatusWarming       Status = "warming"
	StatusOptimized     Status = "optimized"
	StatusTUNUsable     Status = "tun-usable"
	StatusBypassReady   Status = "bypass-ready"
	StatusBypassApplied Status = "bypass-applied"
	StatusRelay         Status = "relay"
	StatusRollback      Status = "rollback"
	StatusFailed        Status = "failed"
)

type Action string

const (
	ActionWatch       Action = "watch"
	ActionReprobe     Action = "reprobe"
	ActionApplyBypass Action = "apply-bypass"
	ActionClearBypass Action = "clear-bypass"
)

type EvaluateInput struct {
	Path            netdiag.PeerPathReport
	AutoBypass      bool
	ActiveBypass    netdiag.ActiveBypass
	HasActiveBypass bool
	LatestLatency   time.Duration
}

type Decision struct {
	Status    Status
	Action    Action
	Candidate netdiag.EgressCandidate
	Message   string
}

type Options struct {
	AutoBypass    bool
	LatestLatency time.Duration
}

type Result struct {
	PeerTailscaleIP string
	Before          netdiag.PeerPathReport
	After           netdiag.PeerPathReport
	Decision        Decision
	Reprobe         netdiag.ReprobeResult
	ActiveBypass    netdiag.ActiveBypass
	HasActiveBypass bool
	Message         string
}
