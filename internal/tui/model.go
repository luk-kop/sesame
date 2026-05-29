package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sesame/internal/app"
	"sesame/internal/domain"
	"sesame/internal/health"
	"sesame/internal/session"
	"sesame/internal/tui/components"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)
	subtleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	valueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	selectedRowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("238"))
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	unknownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)
)

type activeView int

const (
	viewInstances activeView = iota
	viewTunnels
	viewHealth
)

const detailsTagLimit = 8

type Model struct {
	auth           domain.AuthContext
	inventory      app.InventoryProvider
	identity       app.IdentityProvider
	dependencies   health.DependencyStatus
	result         domain.ListResult
	visible        []domain.Instance
	loading        bool
	err            error
	selected       int
	status         string
	view           activeView
	shellStarting  bool
	searchActive   bool
	searchQuery    string
	tunnelManager  *session.TunnelManager
	tunnelModal    bool
	tunnelField    int
	localPort      string
	remotePort     string
	tunnelSelected int
	quitConfirm    bool
	helpOpen       bool
}

type inventoryLoadedMsg struct {
	result domain.ListResult
	err    error
}

type shellReadyMsg struct {
	cmd *exec.Cmd
	err error
}

type shellEndedMsg struct {
	err error
}

type tunnelStartedMsg struct {
	tunnel domain.Tunnel
	err    error
}

type tunnelTickMsg struct{}

func NewModel(auth domain.AuthContext, inventory app.InventoryProvider, identity app.IdentityProvider, dependencies health.DependencyStatus) Model {
	return Model{
		auth:          auth,
		inventory:     inventory,
		identity:      identity,
		dependencies:  dependencies,
		loading:       true,
		tunnelManager: session.NewTunnelManager(auth, nil),
		localPort:     "10022",
		remotePort:    "22",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadInventory(), tunnelTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.helpOpen {
			return m.updateHelp(msg)
		}
		if m.quitConfirm {
			return m.updateQuitConfirm(msg)
		}
		if m.tunnelModal {
			return m.updateTunnelModal(msg)
		}
		if m.searchActive {
			return m.updateSearch(msg)
		}
		switch msg.String() {
		case "?":
			m.helpOpen = true
			return m, nil
		case "q", "ctrl+c":
			return m.requestQuit()
		case "t":
			m.view = viewTunnels
			m.status = ""
		case "h":
			m.view = viewHealth
			m.status = ""
		case "esc":
			if m.view == viewTunnels || m.view == viewHealth {
				m.view = viewInstances
			}
		case "r":
			if m.view == viewTunnels || m.view == viewHealth {
				return m, nil
			}
			m.loading = true
			m.err = nil
			m.status = ""
			return m, m.loadInventory()
		case "enter":
			if m.view == viewTunnels || m.view == viewHealth {
				return m, nil
			}
			return m.startShell()
		case "f":
			if m.view == viewTunnels || m.view == viewHealth {
				return m, nil
			}
			return m.openTunnelModal()
		case "/":
			if m.view == viewTunnels || m.view == viewHealth {
				return m, nil
			}
			m.searchActive = true
			m.status = ""
		case "x":
			if m.view == viewTunnels {
				return m.stopSelectedTunnel()
			}
		case "c":
			if m.view == viewTunnels {
				return m.clearFinishedTunnels()
			}
		case "up", "k":
			if m.view == viewTunnels {
				if m.tunnelSelected > 0 {
					m.tunnelSelected--
				}
			} else if m.view == viewInstances && m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.view == viewTunnels {
				if m.tunnelSelected < len(m.tunnels())-1 {
					m.tunnelSelected++
				}
			} else if m.view == viewInstances && m.selected < len(m.visible)-1 {
				m.selected++
			}
		}
	case inventoryLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.result = msg.result
			m.applySearch(m.selectedInstanceID())
		}
	case shellReadyMsg:
		m.shellStarting = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = "shell session running"
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			return shellEndedMsg{err: err}
		})
	case shellEndedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("shell session failed: %v", msg.err)
		} else {
			m.status = "shell session ended"
		}
	case tunnelStartedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("tunnel failed: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("tunnel running: localhost:%d -> %s:%d", msg.tunnel.LocalPort, msg.tunnel.InstanceID, msg.tunnel.RemotePort)
		}
	case tunnelTickMsg:
		return m, tunnelTick()
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	switch {
	case m.view == viewHealth:
		b.WriteString(m.renderHealth())
	case m.view == viewTunnels:
		b.WriteString(m.renderTunnels())
	case m.loading:
		b.WriteString("Loading instances...\n")
	case m.err != nil:
		fmt.Fprintf(&b, "Error: %v\n", m.err)
	case len(m.visible) == 0:
		b.WriteString("No instances found.\n")
	default:
		b.WriteString(m.renderInstances())
	}
	if m.tunnelModal {
		b.WriteString("\n")
		b.WriteString(m.renderTunnelModal())
	}
	if m.quitConfirm {
		b.WriteString("\n")
		b.WriteString(m.renderQuitConfirm())
	}
	if m.helpOpen {
		b.WriteString("\n")
		b.WriteString(m.renderHelp())
	}

	fmt.Fprintf(&b, "\n%s\n", footerStyle.Render(m.footer()))
	return b.String()
}

func (m Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "?", "esc", "q", "enter":
		m.helpOpen = false
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) requestQuit() (tea.Model, tea.Cmd) {
	if m.tunnelManager != nil && m.tunnelManager.HasActive() {
		m.quitConfirm = true
		m.status = "active tunnels are still running"
		return m, nil
	}
	return m, tea.Quit
}

func (m Model) updateQuitConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.quitConfirm = false
		m.status = "quit cancelled"
		return m, nil
	case tea.KeyEnter:
		if m.tunnelManager != nil {
			m.tunnelManager.StopAll()
		}
		return m, tea.Quit
	}
	switch msg.String() {
	case "y":
		if m.tunnelManager != nil {
			m.tunnelManager.StopAll()
		}
		return m, tea.Quit
	case "n":
		m.quitConfirm = false
		m.status = "quit cancelled"
		return m, nil
	}
	return m, nil
}

func (m Model) stopSelectedTunnel() (tea.Model, tea.Cmd) {
	tunnels := m.tunnels()
	if len(tunnels) == 0 {
		m.status = "no tunnels to stop"
		return m, nil
	}
	if m.tunnelSelected >= len(tunnels) {
		m.tunnelSelected = len(tunnels) - 1
	}
	tunnel := tunnels[m.tunnelSelected]
	if tunnel.State == domain.TunnelStateStopped || tunnel.State == domain.TunnelStateFailed {
		m.status = fmt.Sprintf("tunnel already finished: %s", tunnel.ID)
		return m, nil
	}
	if err := m.tunnelManager.Stop(tunnel.ID); err != nil {
		m.status = fmt.Sprintf("stop tunnel failed: %v", err)
		return m, nil
	}
	m.status = fmt.Sprintf("stopping tunnel: %s", tunnel.ID)
	return m, nil
}

func (m Model) clearFinishedTunnels() (tea.Model, tea.Cmd) {
	m.tunnelManager.ClearFinished()
	tunnels := m.tunnels()
	if m.tunnelSelected >= len(tunnels) {
		m.tunnelSelected = max(0, len(tunnels)-1)
	}
	m.status = "cleared finished tunnels"
	return m, nil
}

func (m Model) openTunnelModal() (tea.Model, tea.Cmd) {
	if m.loading || m.err != nil || len(m.visible) == 0 {
		return m, nil
	}
	if warning := m.dependencyWarning(); warning != "" {
		m.status = fmt.Sprintf("tunnel unavailable: %s", warning)
		return m, nil
	}
	target := m.visible[m.selected]
	if target.SSMStatus != domain.SSMStatusOnline {
		m.status = fmt.Sprintf("tunnel unavailable: instance %s is not SSM online (%s)", target.ID, target.SSMStatus)
		return m, nil
	}
	m.tunnelModal = true
	m.tunnelField = 0
	m.status = fmt.Sprintf("configure tunnel for %s", target.ID)
	return m, nil
}

func (m Model) startShell() (tea.Model, tea.Cmd) {
	if m.loading || m.err != nil || len(m.visible) == 0 {
		return m, nil
	}
	if warning := m.dependencyWarning(); warning != "" {
		m.status = fmt.Sprintf("shell unavailable: %s", warning)
		return m, nil
	}
	target := m.visible[m.selected]
	if target.SSMStatus != domain.SSMStatusOnline {
		m.status = fmt.Sprintf("shell unavailable: instance %s is not SSM online (%s)", target.ID, target.SSMStatus)
		return m, nil
	}
	m.status = fmt.Sprintf("starting shell session for %s", target.ID)
	m.shellStarting = true
	return m, m.preflightShell(target.ID)
}

func (m Model) loadInventory() tea.Cmd {
	return func() tea.Msg {
		result, err := app.ListInstances(context.Background(), m.auth, m.inventory, m.identity, app.ListFilters{})
		return inventoryLoadedMsg{result: result, err: err}
	}
}

func tunnelTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tunnelTickMsg{}
	})
}

func (m Model) preflightShell(instanceID string) tea.Cmd {
	return func() tea.Msg {
		target, _, err := app.PreflightSession(context.Background(), m.inventory, m.identity, instanceID, app.PreflightOptions{})
		if err != nil {
			return shellReadyMsg{err: err}
		}
		starter := session.AwsCliStarter{Auth: m.auth}
		return shellReadyMsg{cmd: starter.ShellCommand(context.Background(), target)}
	}
}

func (m Model) updateTunnelModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.tunnelModal = false
		m.status = "tunnel cancelled"
		return m, nil
	case tea.KeyTab:
		m.tunnelField = (m.tunnelField + 1) % 2
		return m, nil
	case tea.KeyBackspace:
		if m.tunnelField == 0 && m.localPort != "" {
			m.localPort = m.localPort[:len(m.localPort)-1]
		}
		if m.tunnelField == 1 && m.remotePort != "" {
			m.remotePort = m.remotePort[:len(m.remotePort)-1]
		}
		return m, nil
	case tea.KeyEnter:
		return m.startTunnel()
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if r < '0' || r > '9' {
				continue
			}
			if m.tunnelField == 0 {
				m.localPort += string(r)
			} else {
				m.remotePort += string(r)
			}
		}
	}
	return m, nil
}

func (m Model) startTunnel() (tea.Model, tea.Cmd) {
	localPort, err := strconv.Atoi(m.localPort)
	if err != nil {
		m.status = "invalid local port"
		return m, nil
	}
	remotePort, err := strconv.Atoi(m.remotePort)
	if err != nil {
		m.status = "invalid remote port"
		return m, nil
	}
	if err := session.ValidatePort(localPort, "local-port"); err != nil {
		m.status = err.Error()
		return m, nil
	}
	if err := session.ValidatePort(remotePort, "remote-port"); err != nil {
		m.status = err.Error()
		return m, nil
	}

	target := m.visible[m.selected]
	m.tunnelModal = false
	m.status = fmt.Sprintf("starting tunnel for %s", target.ID)
	return m, m.startTunnelCmd(target.ID, localPort, remotePort)
}

func (m Model) startTunnelCmd(instanceID string, localPort, remotePort int) tea.Cmd {
	return func() tea.Msg {
		target, _, err := app.PreflightSession(context.Background(), m.inventory, m.identity, instanceID, app.PreflightOptions{})
		if err != nil {
			return tunnelStartedMsg{err: err}
		}
		tunnel, err := m.tunnelManager.Start(context.Background(), target, localPort, remotePort)
		return tunnelStartedMsg{tunnel: tunnel, err: err}
	}
}

func (m Model) renderInstances() string {
	list := m.renderInstanceTable()
	details := m.renderDetails()
	if details == "" {
		return panelStyle.Render(list)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, panelStyle.Render(list), "  ", panelStyle.Render(details))
}

func (m Model) renderHeader() string {
	var lines []string
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Center, titleStyle.Render("Sesame"), "  ", subtleStyle.Render("AWS SSM Session Manager")))

	context := []string{
		renderKV("Auth", m.authLabel()),
		renderKV("Region", emptyText(m.auth.Region)),
	}
	if m.result.Account != "" {
		context = append(context, renderKV("Account", m.result.Account))
	}
	if m.result.ARN != "" {
		context = append(context, renderKV("ARN", trimMiddle(m.result.ARN, 72)))
	}
	lines = append(lines, strings.Join(context, "  "))

	var signals []string
	if warning := m.dependencyWarning(); warning != "" {
		signals = append(signals, warnStyle.Render("READ-ONLY")+" "+warning)
	}
	if len(m.result.Warnings) > 0 {
		signals = append(signals, warnStyle.Render(fmt.Sprintf("%d warning(s)", len(m.result.Warnings))))
	}
	if m.status != "" {
		signals = append(signals, renderKV("Status", m.status))
	}
	if m.searchActive || m.searchQuery != "" {
		signals = append(signals, renderKV("Search", "/"+m.searchQuery))
	}
	if m.tunnelManager != nil {
		tunnels := m.tunnelManager.List()
		if len(tunnels) > 0 {
			signals = append(signals, renderKV("Tunnels", strconv.Itoa(len(tunnels))))
		}
	}
	if len(signals) > 0 {
		lines = append(lines, strings.Join(signals, "  "))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderInstanceTable() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", headerStyle.Render("Instances"))
	fmt.Fprintf(&b, "%-2s%-22s  %-19s  %-10s  %-13s  %-15s\n", "", "Name", "Instance ID", "State", "SSM", "Private IP")
	fmt.Fprintf(&b, "%s\n", subtleStyle.Render(strings.Repeat("─", 88)))
	for i, inst := range m.visible {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		row := fmt.Sprintf("%-2s%-22s  %-19s  %-10s  %-13s  %-15s",
			cursor,
			trim(inst.Name, 22),
			trim(inst.ID, 19),
			trim(inst.State, 12),
			trim(string(inst.SSMStatus), 13),
			trim(inst.PrivateIP, 15),
		)
		if i == m.selected {
			row = selectedRowStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderDetails() string {
	inst, ok := m.selectedInstance()
	if !ok {
		return ""
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Details"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s\n", renderKV("Name", emptyText(inst.Name)))
	fmt.Fprintf(&b, "%s\n", renderKV("Instance ID", inst.ID))
	fmt.Fprintf(&b, "%s\n", renderKV("Type", emptyText(inst.Type)))
	fmt.Fprintf(&b, "%s\n", renderKV("State", emptyText(inst.State)))
	fmt.Fprintf(&b, "%s\n", renderKV("Private IP", emptyText(inst.PrivateIP)))
	fmt.Fprintf(&b, "%s\n", renderKV("Public IP", emptyText(inst.PublicIP)))
	fmt.Fprintf(&b, "%s\n", renderKV("Region", emptyText(inst.Region)))
	fmt.Fprintf(&b, "%s\n", renderKV("SSM", styledSSMStatus(inst.SSMStatus)))
	fmt.Fprintf(&b, "%s\n", renderKV("Agent version", emptyText(inst.Agent.Version)))
	fmt.Fprintf(&b, "%s\n", renderKV("Agent last ping", lastPingText(inst.Agent.LastPingUnixTime)))
	fmt.Fprintf(&b, "%s\n", renderKV("Platform", emptyText(inst.Agent.PlatformType)))
	if inst.SSMStatus != domain.SSMStatusOnline {
		fmt.Fprintf(&b, "%s\n", renderKV("Action status", fmt.Sprintf("shell/tunnel unavailable while SSM is %s", inst.SSMStatus)))
	}
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Tags"))
	b.WriteString("\n")
	if len(inst.Tags) == 0 {
		b.WriteString("  -\n")
		return b.String()
	}
	keys := make([]string, 0, len(inst.Tags))
	for key := range inst.Tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	visibleKeys := keys
	if len(visibleKeys) > detailsTagLimit {
		visibleKeys = visibleKeys[:detailsTagLimit]
	}
	for _, key := range visibleKeys {
		fmt.Fprintf(&b, "  %s\n", renderKV(key, inst.Tags[key]))
	}
	if hidden := len(keys) - len(visibleKeys); hidden > 0 {
		fmt.Fprintf(&b, "  %s\n", subtleStyle.Render(fmt.Sprintf("+ %d more tags", hidden)))
	}
	return b.String()
}

func (m Model) renderTunnelModal() string {
	localMarker := " "
	remoteMarker := " "
	if m.tunnelField == 0 {
		localMarker = ">"
	} else {
		remoteMarker = ">"
	}
	return fmt.Sprintf("Port forwarding\n%s local port:  %s\n%s remote port: %s\nEnter start  Tab switch  Esc cancel\n", localMarker, m.localPort, remoteMarker, m.remotePort)
}

func (m Model) renderQuitConfirm() string {
	return "Active tunnels are running.\nEnter/y stop tunnels and quit  Esc/n cancel\n"
}

func (m Model) renderHelp() string {
	return strings.Join([]string{
		"Help",
		"",
		"Global",
		"  q / Ctrl+C  quit",
		"  ?           toggle this help",
		"",
		"Instances",
		"  up/down or k/j  move selection",
		"  /               search",
		"  r               refresh inventory",
		"  Enter           start shell session",
		"  f               open port forwarding modal",
		"  t               show tunnels",
		"  h               show health",
		"",
		"Search",
		"  type            filter visible instances",
		"  Esc             clear query, then close search",
		"  Enter           start shell for selected result",
		"",
		"Port forwarding modal",
		"  Tab             switch field",
		"  Enter           start tunnel",
		"  Esc             cancel",
		"",
		"Tunnels",
		"  up/down or k/j  move selection",
		"  x               stop selected tunnel",
		"  c               clear finished tunnels",
		"  Esc             return to instances",
		"",
		"Health",
		"  h               show health checklist",
		"  Esc             return to instances",
		"",
		"Close help with ?, Esc, Enter, or q.",
	}, "\n") + "\n"
}

func (m Model) renderHealth() string {
	var b strings.Builder
	b.WriteString("Health\n")
	fmt.Fprintf(&b, "%s aws CLI\n", checkMark(m.dependencies.AWSCLI))
	fmt.Fprintf(&b, "%s session-manager-plugin\n", checkMark(m.dependencies.SessionManagerPlugin))
	fmt.Fprintf(&b, "%s auth mode: %s\n", checkMark(m.auth.Mode != ""), m.authLabel())
	fmt.Fprintf(&b, "%s region: %s\n", checkMark(m.auth.Region != ""), emptyText(m.auth.Region))
	fmt.Fprintf(&b, "%s sts account: %s\n", checkMark(m.result.Account != ""), emptyText(m.result.Account))
	fmt.Fprintf(&b, "%s sts arn: %s\n", checkMark(m.result.ARN != ""), emptyText(m.result.ARN))
	if m.loading {
		b.WriteString("- inventory: loading\n")
	} else if m.err != nil {
		fmt.Fprintf(&b, "x inventory: %v\n", m.err)
	} else {
		fmt.Fprintf(&b, "%s inventory instances: %d\n", checkMark(true), len(m.result.Instances))
	}
	if len(m.result.Warnings) == 0 {
		b.WriteString("ok warnings: none\n")
	} else {
		fmt.Fprintf(&b, "! warnings: %d\n", len(m.result.Warnings))
		for _, warning := range m.result.Warnings {
			fmt.Fprintf(&b, "  %s: %s\n", warning.Code, warning.Message)
		}
	}
	if m.tunnelManager != nil && m.tunnelManager.HasActive() {
		b.WriteString("! active tunnels block profile/region switch\n")
	}
	return b.String()
}

func (m Model) renderTunnels() string {
	tunnels := m.tunnels()
	if len(tunnels) == 0 {
		return "No tunnels.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-8s  %-20s  %-10s  %-10s  %-12s\n", "STATE", "INSTANCE ID", "LOCAL", "REMOTE", "PROFILE")
	for i, tunnel := range tunnels {
		cursor := " "
		if i == m.tunnelSelected {
			cursor = ">"
		}
		errText := ""
		if tunnel.Err != nil && !errors.Is(tunnel.Err, context.Canceled) {
			errText = " " + trim(tunnel.Err.Error(), 30)
		}
		fmt.Fprintf(&b, "%s %-8s  %-20s  %-10d  %-10d  %-12s%s\n",
			cursor,
			tunnel.State,
			trim(tunnel.InstanceID, 20),
			tunnel.LocalPort,
			tunnel.RemotePort,
			trim(tunnel.Profile, 12),
			errText,
		)
	}
	return b.String()
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.applySearch("")
			return m, nil
		}
		m.searchActive = false
		return m, nil
	case tea.KeyBackspace:
		if m.searchQuery != "" {
			keepID := m.selectedInstanceID()
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.applySearch(keepID)
		}
		return m, nil
	case tea.KeyEnter:
		return m.startShell()
	}

	if msg.Type == tea.KeyRunes {
		keepID := m.selectedInstanceID()
		m.searchQuery += msg.String()
		m.applySearch(keepID)
	}
	return m, nil
}

func (m *Model) applySearch(preferredID string) {
	if m.searchQuery == "" {
		m.visible = append([]domain.Instance(nil), m.result.Instances...)
	} else {
		m.visible = m.visible[:0]
		for _, inst := range m.result.Instances {
			if matchesSearch(inst, m.searchQuery) {
				m.visible = append(m.visible, inst)
			}
		}
	}
	m.selected = 0
	if preferredID != "" {
		for i, inst := range m.visible {
			if inst.ID == preferredID {
				m.selected = i
				return
			}
		}
	}
	if len(m.visible) == 0 {
		m.selected = 0
	}
}

func matchesSearch(inst domain.Instance, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	fields := []string{
		inst.Name,
		inst.ID,
		inst.PrivateIP,
		inst.PublicIP,
		inst.State,
		string(inst.SSMStatus),
		inst.Region,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	for key, value := range inst.Tags {
		if strings.Contains(strings.ToLower(key), query) || strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func (m Model) selectedInstanceID() string {
	if len(m.visible) == 0 || m.selected < 0 || m.selected >= len(m.visible) {
		return ""
	}
	return m.visible[m.selected].ID
}

func (m Model) selectedInstance() (domain.Instance, bool) {
	if len(m.visible) == 0 || m.selected < 0 || m.selected >= len(m.visible) {
		return domain.Instance{}, false
	}
	return m.visible[m.selected], true
}

func (m Model) footer() string {
	if m.helpOpen {
		return "?/Esc/Enter/q close help"
	}
	if m.quitConfirm {
		return "Enter/y stop tunnels and quit  Esc/n cancel"
	}
	if m.tunnelModal {
		return "Enter start  Tab switch field  Esc cancel"
	}
	if m.view == viewTunnels {
		return "↑↓ move  x stop  c clear finished  Esc instances  q quit"
	}
	if m.view == viewHealth {
		return "Esc instances  q quit"
	}
	if m.searchActive {
		return "type to search  Esc clear/close  Enter shell"
	}
	return components.Footer()
}

func (m Model) tunnels() []domain.Tunnel {
	if m.tunnelManager == nil {
		return nil
	}
	tunnels := m.tunnelManager.List()
	if m.tunnelSelected >= len(tunnels) {
		m.tunnelSelected = max(0, len(tunnels)-1)
	}
	return tunnels
}

func (m Model) authLabel() string {
	if m.auth.Mode == domain.AuthModeProfileActive && m.auth.Profile != "" {
		return fmt.Sprintf("%s %s", m.auth.Mode, m.auth.Profile)
	}
	return string(m.auth.Mode)
}

func (m Model) dependencyWarning() string {
	switch {
	case m.dependencies.AWSCLI && m.dependencies.SessionManagerPlugin:
		return ""
	case !m.dependencies.AWSCLI:
		return "aws CLI not found"
	case !m.dependencies.SessionManagerPlugin:
		return "session-manager-plugin not found"
	default:
		return ""
	}
}

func trim(value string, width int) string {
	if value == "" {
		value = "-"
	}
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func emptyText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func lastPingText(unix int64) string {
	if unix == 0 {
		return "-"
	}
	return time.Unix(unix, 0).UTC().Format("2006-01-02 15:04:05 UTC")
}

func checkMark(ok bool) string {
	if ok {
		return "ok"
	}
	return "x"
}

func renderKV(label, value string) string {
	return labelStyle.Render(label+":") + " " + valueStyle.Render(value)
}

func styledSSMStatus(status domain.SSMStatus) string {
	switch status {
	case domain.SSMStatusOnline:
		return okStyle.Render(string(status))
	case domain.SSMStatusConnectionLost, domain.SSMStatusNotManaged:
		return warnStyle.Render(string(status))
	case domain.SSMStatusError:
		return errorStyle.Render(string(status))
	default:
		return unknownStyle.Render(string(status))
	}
}

func trimMiddle(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 7 {
		return trim(value, width)
	}
	left := (width - 3) / 2
	right := width - 3 - left
	return value[:left] + "..." + value[len(value)-right:]
}
