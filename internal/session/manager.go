package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"sesame/internal/domain"
)

type CommandFactory func(ctx context.Context, target domain.Instance, localPort, remotePort int) *exec.Cmd
type PortChecker func(port int) error

const tunnelStopGracePeriod = 2 * time.Second
const tunnelOutputLimit = 8 * 1024

type TunnelManager struct {
	mu       sync.Mutex
	auth     domain.AuthContext
	commands CommandFactory
	ports    PortChecker
	tunnels  map[string]*managedTunnel
}

type managedTunnel struct {
	tunnel   domain.Tunnel
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	output   *tailBuffer
	outputWG sync.WaitGroup
	stopping bool
}

func NewTunnelManager(auth domain.AuthContext, commands CommandFactory) *TunnelManager {
	return NewTunnelManagerWithPortChecker(auth, commands, checkLocalPortAvailable)
}

func NewTunnelManagerWithPortChecker(auth domain.AuthContext, commands CommandFactory, ports PortChecker) *TunnelManager {
	if commands == nil {
		starter := AwsCliStarter{Auth: auth}
		commands = starter.TunnelCommand
	}
	if ports == nil {
		ports = checkLocalPortAvailable
	}
	return &TunnelManager{
		auth:     auth,
		commands: commands,
		ports:    ports,
		tunnels:  map[string]*managedTunnel{},
	}
}

func (m *TunnelManager) Start(ctx context.Context, target domain.Instance, localPort, remotePort int) (domain.Tunnel, error) {
	if err := ValidatePort(localPort, "local-port"); err != nil {
		return domain.Tunnel{}, err
	}
	if err := ValidatePort(remotePort, "remote-port"); err != nil {
		return domain.Tunnel{}, err
	}
	if err := m.ports(localPort); err != nil {
		return domain.Tunnel{}, err
	}

	key := tunnelKey(m.auth, target.ID, localPort)
	m.mu.Lock()
	if existing, ok := m.tunnels[key]; ok && existing.tunnel.State != domain.TunnelStateStopped && existing.tunnel.State != domain.TunnelStateFailed {
		m.mu.Unlock()
		return domain.Tunnel{}, fmt.Errorf("tunnel already exists for %s local port %d", target.ID, localPort)
	}

	tunnelCtx, cancel := context.WithCancel(ctx)
	tunnel := domain.Tunnel{
		ID:         key,
		InstanceID: target.ID,
		Region:     m.auth.Region,
		Profile:    m.auth.Profile,
		LocalPort:  localPort,
		RemotePort: remotePort,
		StartedAt:  time.Now(),
		State:      domain.TunnelStateStarting,
	}
	cmd := m.commands(tunnelCtx, target, localPort, remotePort)
	managed := &managedTunnel{tunnel: tunnel, cancel: cancel, cmd: cmd, output: newTailBuffer(tunnelOutputLimit)}
	if err := managed.captureOutput(); err != nil {
		cancel()
		m.mu.Unlock()
		return domain.Tunnel{}, err
	}
	m.tunnels[key] = managed
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		cancel()
		managed.outputWG.Wait()
		m.mu.Lock()
		managed.tunnel.State = domain.TunnelStateFailed
		managed.tunnel.Err = err
		managed.tunnel.Output = managed.output.String()
		m.mu.Unlock()
		return managed.tunnel, err
	}

	m.mu.Lock()
	managed.tunnel.State = domain.TunnelStateRunning
	started := managed.tunnel
	m.mu.Unlock()

	go m.wait(key, cmd)
	return started, nil
}

func (m *TunnelManager) Stop(id string) error {
	m.mu.Lock()
	managed, ok := m.tunnels[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("tunnel not found: %s", id)
	}
	if managed.tunnel.State == domain.TunnelStateStopped || managed.tunnel.State == domain.TunnelStateFailed {
		m.mu.Unlock()
		return nil
	}
	managed.stopping = true
	cmd := managed.cmd
	cancel := managed.cancel
	m.mu.Unlock()

	stopProcessGracefully(cmd, cancel)
	return nil
}

func (m *TunnelManager) StopAll() {
	for _, tunnel := range m.List() {
		if tunnel.State == domain.TunnelStateStarting || tunnel.State == domain.TunnelStateRunning {
			_ = m.Stop(tunnel.ID)
		}
	}
}

func (m *TunnelManager) List() []domain.Tunnel {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]domain.Tunnel, 0, len(m.tunnels))
	for _, managed := range m.tunnels {
		out = append(out, managed.tunnel)
	}
	return out
}

func (m *TunnelManager) HasActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, managed := range m.tunnels {
		if managed.tunnel.State == domain.TunnelStateStarting || managed.tunnel.State == domain.TunnelStateRunning {
			return true
		}
	}
	return false
}

func (m *TunnelManager) ClearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, managed := range m.tunnels {
		if managed.tunnel.State == domain.TunnelStateStopped || managed.tunnel.State == domain.TunnelStateFailed {
			delete(m.tunnels, id)
		}
	}
}

func (m *TunnelManager) wait(id string, cmd *exec.Cmd) {
	err := cmd.Wait()
	m.mu.Lock()

	managed, ok := m.tunnels[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	managed.outputWG.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	managed, ok = m.tunnels[id]
	if !ok {
		return
	}
	managed.tunnel.Output = managed.output.String()
	if managed.stopping {
		managed.tunnel.State = domain.TunnelStateStopped
		return
	}
	if err != nil {
		managed.tunnel.State = domain.TunnelStateFailed
		managed.tunnel.Err = err
		return
	}
	managed.tunnel.State = domain.TunnelStateStopped
}

func (t *managedTunnel) captureOutput() error {
	if t.cmd.Stdout == nil {
		stdout, err := t.cmd.StdoutPipe()
		if err != nil {
			return err
		}
		t.outputWG.Add(1)
		go func() {
			defer t.outputWG.Done()
			_, _ = io.Copy(t.output, stdout)
		}()
	}
	if t.cmd.Stderr == nil {
		stderr, err := t.cmd.StderrPipe()
		if err != nil {
			return err
		}
		t.outputWG.Add(1)
		go func() {
			defer t.outputWG.Done()
			_, _ = io.Copy(t.output, stderr)
		}()
	}
	return nil
}

func stopProcessGracefully(cmd *exec.Cmd, cancel context.CancelFunc) {
	if cmd == nil || cmd.Process == nil {
		cancel()
		return
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		cancel()
		_ = cmd.Process.Kill()
		return
	}

	go func() {
		timer := time.NewTimer(tunnelStopGracePeriod)
		defer timer.Stop()
		<-timer.C
		cancel()
		_ = cmd.Process.Kill()
	}()
}

func tunnelKey(auth domain.AuthContext, instanceID string, localPort int) string {
	return fmt.Sprintf("%s:%s:%s:%d", auth.Profile, auth.Region, instanceID, localPort)
}

type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, p...)
	if b.limit > 0 && len(b.data) > b.limit {
		b.data = append([]byte(nil), b.data[len(b.data)-b.limit:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return string(b.data)
}
