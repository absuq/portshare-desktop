package localhostbridge

import (
	"context"
	"reflect"
	"sort"
	"sync"
)

type Scanner interface {
	Scan(context.Context) ([]ListeningPort, error)
}

type BridgeRunner interface {
	Start(context.Context) error
	Close() error
}

type Config struct {
	Scanner          Scanner
	LocalTailscaleIP string
	AllowedPeerIPs   []string
	NewBridge        func(BridgePlan) BridgeRunner
}

type Controller struct {
	mu               sync.Mutex
	scanner          Scanner
	localTailscaleIP string
	allowedPeerIPs   []string
	newBridge        func(BridgePlan) BridgeRunner
	active           map[int]activeBridge
	conflicts        []int
}

type activeBridge struct {
	plan   BridgePlan
	bridge BridgeRunner
}

func NewController(config Config) *Controller {
	newBridge := config.NewBridge
	if newBridge == nil {
		newBridge = func(plan BridgePlan) BridgeRunner { return NewBridge(plan) }
	}
	return &Controller{
		scanner:          config.Scanner,
		localTailscaleIP: config.LocalTailscaleIP,
		allowedPeerIPs:   normalizeAllowedPeerIPs(config.AllowedPeerIPs),
		newBridge:        newBridge,
		active:           make(map[int]activeBridge),
	}
}

func (c *Controller) SetLocalTailscaleIP(ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.localTailscaleIP = ip
}

func (c *Controller) SetAllowedPeers(peers []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allowedPeerIPs = normalizeAllowedPeerIPs(peers)
}

func (c *Controller) Refresh(ctx context.Context) error {
	if c == nil || c.scanner == nil {
		return nil
	}
	listeners, err := c.scanner.Scan(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	localIP := c.localTailscaleIP
	allowed := append([]string(nil), c.allowedPeerIPs...)
	c.mu.Unlock()

	result := BuildPlanResult(PlanInput{
		LocalTailscaleIP: localIP,
		AllowedPeerIPs:   allowed,
		Listeners:        listeners,
	})
	desired := make(map[int]BridgePlan, len(result.Bridges))
	for _, plan := range result.Bridges {
		desired[plan.Port] = plan
	}
	conflicts := conflictPorts(result.Conflicts)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.conflicts = conflicts

	for port, active := range c.active {
		plan, ok := desired[port]
		if !ok || !bridgePlansEqual(active.plan, plan) {
			_ = active.bridge.Close()
			delete(c.active, port)
		}
	}
	for port, plan := range desired {
		if _, ok := c.active[port]; ok {
			continue
		}
		bridge := c.newBridge(plan)
		if err := bridge.Start(ctx); err != nil {
			return err
		}
		c.active[port] = activeBridge{plan: plan, bridge: bridge}
	}
	return nil
}

func (c *Controller) ActivePorts() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	ports := make([]int, 0, len(c.active))
	for port := range c.active {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func (c *Controller) ConflictPorts() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.conflicts...)
}

func (c *Controller) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	active := c.active
	c.active = make(map[int]activeBridge)
	c.conflicts = nil
	c.mu.Unlock()
	for _, bridge := range active {
		_ = bridge.bridge.Close()
	}
	return nil
}

func conflictPorts(conflicts []Conflict) []int {
	ports := make([]int, 0, len(conflicts))
	for _, conflict := range conflicts {
		ports = append(ports, conflict.Port)
	}
	sort.Ints(ports)
	return ports
}

func bridgePlansEqual(a, b BridgePlan) bool {
	return a.Port == b.Port &&
		a.ListenAddress == b.ListenAddress &&
		a.TargetAddress == b.TargetAddress &&
		reflect.DeepEqual(a.AllowedPeerIPs, b.AllowedPeerIPs)
}
