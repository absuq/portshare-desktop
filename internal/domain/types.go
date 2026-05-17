package domain

import "time"

type ShareMode string

const (
	ModeTailnet ShareMode = "tailnet"
	ModePublic  ShareMode = "public"
)

func (m ShareMode) String() string {
	return string(m)
}

type Language string

const (
	LanguageChinese Language = "zh-CN"
	LanguageEnglish Language = "en-US"
)

type LocalService struct {
	ID          string
	Name        string
	Scheme      string
	Host        string
	Port        int
	Title       string
	Discovered  bool
	LastChecked time.Time
}

type ShareStatus string

const (
	ShareStopped  ShareStatus = "stopped"
	ShareStarting ShareStatus = "starting"
	ShareActive   ShareStatus = "active"
	ShareError    ShareStatus = "error"
)

type Share struct {
	ID          string
	ServiceID   string
	Provider    string
	Mode        ShareMode
	LocalURL    string
	PublicURL   string
	Status      ShareStatus
	StartedAt   time.Time
	ExpiresAt   *time.Time
	LastError   string
	LongRunning bool
}
