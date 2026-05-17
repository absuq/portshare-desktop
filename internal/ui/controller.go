package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/absuq/portshare-desktop/internal/domain"
)

var (
	ErrNoServiceSelected = errors.New("no service selected")
	ErrNoActiveShare     = errors.New("no active share")
)

type Manager interface {
	PublishTailnet(context.Context, domain.LocalService) (domain.Share, error)
	PublishPublic(context.Context, domain.LocalService, time.Duration, bool) (domain.Share, error)
	Stop(context.Context, domain.Share, string) error
	StopAllPublic(context.Context) error
	StopAll(context.Context) error
	Status(context.Context) ([]domain.Share, error)
}

type Discovery interface {
	ScanCommon(time.Duration) []domain.LocalService
	Probe(string, time.Duration) (domain.LocalService, error)
}

type DiscoveryFuncs struct {
	ScanCommonFunc func(time.Duration) []domain.LocalService
	ProbeFunc      func(string, time.Duration) (domain.LocalService, error)
}

func (d DiscoveryFuncs) ScanCommon(timeout time.Duration) []domain.LocalService {
	if d.ScanCommonFunc == nil {
		return nil
	}
	return d.ScanCommonFunc(timeout)
}

func (d DiscoveryFuncs) Probe(rawURL string, timeout time.Duration) (domain.LocalService, error) {
	if d.ProbeFunc == nil {
		return domain.LocalService{}, errors.New("probe function is not configured")
	}
	return d.ProbeFunc(rawURL, timeout)
}

type Controller struct {
	manager   Manager
	discovery Discovery
	timeout   time.Duration

	services []domain.LocalService
	shares   []domain.Share
	selected int
	message  string
}

type State struct {
	Services     []ServiceItem
	Selected     int
	HasSelection bool
	Message      string
}

type ServiceItem struct {
	ID         string
	Name       string
	LocalURL   string
	StatusText string
	TailnetURL string
	PublicURL  string
	Service    domain.LocalService
}

func NewController(deps Dependencies) *Controller {
	timeout := deps.Timeout
	if timeout == 0 {
		timeout = 500 * time.Millisecond
	}
	return &Controller{
		manager:   deps.Manager,
		discovery: deps.Discovery,
		timeout:   timeout,
		selected:  -1,
	}
}

func (c *Controller) Refresh(ctx context.Context) error {
	if c.discovery != nil {
		c.upsertServices(c.discovery.ScanCommon(c.timeout)...)
	}
	if err := c.refreshShares(ctx); err != nil {
		c.message = "状态刷新失败：" + err.Error()
		return err
	}
	c.ensureSelection()
	if len(c.services) == 0 {
		c.message = "没有发现本地 HTTP/HTTPS 服务"
	} else {
		c.message = fmt.Sprintf("已刷新，发现 %d 个服务", len(c.services))
	}
	return nil
}

func (c *Controller) AddManual(ctx context.Context, raw string) error {
	normalized, err := normalizeLocalURL(raw)
	if err != nil {
		c.message = "服务地址无效：" + err.Error()
		return err
	}
	if c.discovery == nil {
		err := errors.New("service discovery is not configured")
		c.message = "服务探测不可用：" + err.Error()
		return err
	}
	svc, err := c.discovery.Probe(normalized, c.timeout)
	if err != nil {
		c.message = "服务探测失败：" + err.Error()
		return err
	}
	c.upsertServices(svc)
	c.selectByID(svc.ID)
	if err := c.refreshShares(ctx); err != nil {
		c.message = "已添加服务，但状态刷新失败：" + err.Error()
		return err
	}
	c.message = "已添加服务：" + displayName(svc)
	return nil
}

func (c *Controller) Select(index int) {
	if index < 0 || index >= len(c.services) {
		c.selected = -1
		return
	}
	c.selected = index
}

func (c *Controller) PublishTailnet(ctx context.Context) (domain.Share, error) {
	svc, ok := c.selectedService()
	if !ok {
		c.message = "请先选择一个本地服务"
		return domain.Share{}, ErrNoServiceSelected
	}
	share, err := c.manager.PublishTailnet(ctx, svc)
	if err != nil {
		c.message = "tailnet 发布失败：" + err.Error()
		return share, err
	}
	c.upsertShare(share)
	c.message = "已开放到 tailnet：" + share.PublicURL
	return share, nil
}

func (c *Controller) PublishPublic(ctx context.Context, choice PublicChoice) (domain.Share, error) {
	svc, ok := c.selectedService()
	if !ok {
		c.message = "请先选择一个本地服务"
		return domain.Share{}, ErrNoServiceSelected
	}
	share, err := c.manager.PublishPublic(ctx, svc, choice.TTL, choice.LongRunning)
	if err != nil {
		c.message = "公网发布失败：" + err.Error()
		return share, err
	}
	c.upsertShare(share)
	c.message = "已开启公网：" + share.PublicURL
	return share, nil
}

func (c *Controller) StopSelected(ctx context.Context) error {
	svc, ok := c.selectedService()
	if !ok {
		c.message = "请先选择一个本地服务"
		return ErrNoServiceSelected
	}
	active := c.sharesForService(svc.ID)
	if len(active) == 0 {
		c.message = "当前服务没有正在发布的地址"
		return ErrNoActiveShare
	}
	for _, share := range active {
		if err := c.manager.Stop(ctx, share, "manual"); err != nil {
			c.message = "停止发布失败：" + err.Error()
			return err
		}
		c.removeShare(share.ID)
	}
	c.message = "已停止当前服务的发布"
	return nil
}

func (c *Controller) StopAllPublic(ctx context.Context) error {
	if err := c.manager.StopAllPublic(ctx); err != nil {
		c.message = "暂停所有公网失败：" + err.Error()
		return err
	}
	c.removeSharesByMode(domain.ModePublic)
	c.message = "已暂停所有公网发布"
	return nil
}

func (c *Controller) StopAll(ctx context.Context) error {
	if err := c.manager.StopAll(ctx); err != nil {
		c.message = "停止全部发布失败：" + err.Error()
		return err
	}
	c.shares = nil
	c.message = "已停止全部发布"
	return nil
}

func (c *Controller) State() State {
	items := make([]ServiceItem, 0, len(c.services))
	for _, svc := range c.services {
		item := ServiceItem{
			ID:       svc.ID,
			Name:     displayName(svc),
			LocalURL: localURL(svc),
			Service:  svc,
		}
		for _, share := range c.sharesForService(svc.ID) {
			switch share.Mode {
			case domain.ModeTailnet:
				item.TailnetURL = share.PublicURL
			case domain.ModePublic:
				item.PublicURL = share.PublicURL
			}
		}
		item.StatusText = statusText(item)
		items = append(items, item)
	}
	return State{
		Services:     items,
		Selected:     c.selected,
		HasSelection: c.selected >= 0 && c.selected < len(c.services),
		Message:      c.message,
	}
}

func (c *Controller) refreshShares(ctx context.Context) error {
	if c.manager == nil {
		return nil
	}
	shares, err := c.manager.Status(ctx)
	if err != nil {
		return err
	}
	if len(shares) == 0 && len(c.shares) > 0 {
		return nil
	}
	c.shares = append([]domain.Share(nil), shares...)
	return nil
}

func (c *Controller) upsertServices(services ...domain.LocalService) {
	for _, svc := range services {
		found := false
		for i := range c.services {
			if c.services[i].ID == svc.ID {
				c.services[i] = svc
				found = true
				break
			}
		}
		if !found {
			c.services = append(c.services, svc)
		}
	}
}

func (c *Controller) upsertShare(share domain.Share) {
	for i := range c.shares {
		if c.shares[i].ID == share.ID || (c.shares[i].ServiceID == share.ServiceID && c.shares[i].Mode == share.Mode) {
			c.shares[i] = share
			return
		}
	}
	c.shares = append(c.shares, share)
}

func (c *Controller) removeShare(id string) {
	for i := 0; i < len(c.shares); i++ {
		if c.shares[i].ID == id {
			c.shares = append(c.shares[:i], c.shares[i+1:]...)
			i--
		}
	}
}

func (c *Controller) removeSharesByMode(mode domain.ShareMode) {
	for i := 0; i < len(c.shares); i++ {
		if c.shares[i].Mode == mode {
			c.shares = append(c.shares[:i], c.shares[i+1:]...)
			i--
		}
	}
}

func (c *Controller) sharesForService(serviceID string) []domain.Share {
	var shares []domain.Share
	for _, share := range c.shares {
		if share.ServiceID == serviceID && share.Status == domain.ShareActive {
			shares = append(shares, share)
		}
	}
	return shares
}

func (c *Controller) selectedService() (domain.LocalService, bool) {
	if c.selected < 0 || c.selected >= len(c.services) {
		return domain.LocalService{}, false
	}
	return c.services[c.selected], true
}

func (c *Controller) ensureSelection() {
	if len(c.services) == 0 {
		c.selected = -1
		return
	}
	if c.selected < 0 || c.selected >= len(c.services) {
		c.selected = 0
	}
}

func (c *Controller) selectByID(id string) {
	for i, svc := range c.services {
		if svc.ID == id {
			c.selected = i
			return
		}
	}
	c.ensureSelection()
}

func normalizeLocalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("请输入本地服务地址")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("只支持 HTTP/HTTPS")
	}
	if parsed.Hostname() == "" {
		return "", errors.New("缺少主机名")
	}
	if parsed.Port() == "" {
		return "", errors.New("缺少端口")
	}
	return parsed.String(), nil
}

func displayName(svc domain.LocalService) string {
	if svc.Name != "" {
		return svc.Name
	}
	if svc.Title != "" {
		return svc.Title
	}
	return localURL(svc)
}

func localURL(svc domain.LocalService) string {
	return fmt.Sprintf("%s://%s:%d", svc.Scheme, svc.Host, svc.Port)
}

func statusText(item ServiceItem) string {
	switch {
	case item.TailnetURL != "" && item.PublicURL != "":
		return "tailnet 与公网已开放"
	case item.PublicURL != "":
		return "公网已开放"
	case item.TailnetURL != "":
		return "tailnet 已开放"
	default:
		return "未发布"
	}
}
