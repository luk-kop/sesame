package tui

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
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

type instanceSortKey string

const (
	sortNone       instanceSortKey = ""
	sortName       instanceSortKey = "name"
	sortInstanceID instanceSortKey = "instance ID"
	sortState      instanceSortKey = "state"
	sortSSM        instanceSortKey = "SSM"
	sortPrivateIP  instanceSortKey = "private IP"
)

const wideDetailsMinWidth = 132
const wideTableMinWidth = 131
const mediumTableMinWidth = 96
const compactTableMinWidth = 72
const detailsTagLimit = 8
const defaultAppVersion = "dev revision=unknown build_date=unknown"

type tunnelPreset struct {
	Name       string
	LocalPort  string
	RemotePort string
}

var tunnelPresets = []tunnelPreset{
	{Name: "SSH", LocalPort: "10022", RemotePort: "22"},
	{Name: "PostgreSQL", LocalPort: "15432", RemotePort: "5432"},
	{Name: "MySQL", LocalPort: "13306", RemotePort: "3306"},
	{Name: "Redis", LocalPort: "16379", RemotePort: "6379"},
	{Name: "HTTP", LocalPort: "18080", RemotePort: "80"},
	{Name: "HTTPS", LocalPort: "18443", RemotePort: "443"},
}

type Model struct {
	auth             domain.AuthContext
	inventory        app.InventoryProvider
	identity         app.IdentityProvider
	regionProvider   app.RegionProvider
	dependencies     health.DependencyStatus
	appVersion       string
	factory          providerFactory
	profileOptions   []string
	regionOptions    []string
	regionCache      map[regionCacheKey][]string
	result           domain.ListResult
	visible          []domain.Instance
	loading          bool
	err              error
	width            int
	height           int
	selected         int
	wideMode         bool
	sortKey          instanceSortKey
	sortDescending   bool
	status           string
	view             activeView
	shellStarting    bool
	searchActive     bool
	searchQuery      string
	tunnelManager    *session.TunnelManager
	tunnelModal      bool
	profileModal     bool
	profileInput     string
	profileSelected  int
	regionModal      bool
	regionInput      string
	regionSelected   int
	regionInputDirty bool
	regionLoading    bool
	regionLoadError  string
	detailsFocused   bool
	tunnelField      int
	tunnelPreset     int
	localPort        string
	remotePort       string
	tunnelSelected   int
	quitConfirm      bool
	helpOpen         bool
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

type providerFactory func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error)

type regionCacheKey struct {
	Mode    domain.AuthMode
	Profile string
	Region  string
}

type profileChangedMsg struct {
	auth      domain.AuthContext
	inventory app.InventoryProvider
	identity  app.IdentityProvider
	regions   app.RegionProvider
	result    domain.ListResult
	err       error
}

type regionChangedMsg struct {
	auth      domain.AuthContext
	inventory app.InventoryProvider
	identity  app.IdentityProvider
	regions   app.RegionProvider
	result    domain.ListResult
	err       error
}

type regionsLoadedMsg struct {
	key     regionCacheKey
	regions []string
	err     error
}

func NewModel(auth domain.AuthContext, inventory app.InventoryProvider, identity app.IdentityProvider, dependencies health.DependencyStatus) Model {
	return NewModelWithProviderFactory(auth, inventory, identity, nil, dependencies, nil, nil)
}

func NewModelWithProviderFactory(auth domain.AuthContext, inventory app.InventoryProvider, identity app.IdentityProvider, regions app.RegionProvider, dependencies health.DependencyStatus, factory providerFactory, profiles []string) Model {
	return NewModelWithProviderFactoryAndVersion(auth, inventory, identity, regions, dependencies, factory, profiles, defaultAppVersion)
}

func NewModelWithProviderFactoryAndVersion(auth domain.AuthContext, inventory app.InventoryProvider, identity app.IdentityProvider, regions app.RegionProvider, dependencies health.DependencyStatus, factory providerFactory, profiles []string, appVersion string) Model {
	return Model{
		auth:           auth,
		inventory:      inventory,
		identity:       identity,
		regionProvider: regions,
		dependencies:   dependencies,
		appVersion:     normalizeAppVersion(appVersion),
		factory:        factory,
		profileOptions: normalizeProfileOptions(profiles, auth.Profile),
		regionOptions:  normalizeRegionOptions(nil, auth.Region),
		regionCache:    map[regionCacheKey][]string{},
		loading:        true,
		tunnelManager:  session.NewTunnelManager(auth, nil),
		localPort:      "10022",
		remotePort:     "22",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadInventory(), tunnelTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
		if m.profileModal {
			return m.updateProfileModal(msg)
		}
		if m.regionModal {
			return m.updateRegionModal(msg)
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
		case "p":
			return m.openProfileModal()
		case "g":
			return m.openRegionModal()
		case "/":
			if m.view == viewTunnels || m.view == viewHealth {
				return m, nil
			}
			m.searchActive = true
			m.status = ""
		case "w":
			if m.view == viewTunnels || m.view == viewHealth {
				return m, nil
			}
			m.wideMode = !m.wideMode
			if m.isWideTable() {
				m.status = "wide table enabled"
			} else if m.wideMode {
				m.status = "wide table pending wider terminal"
			} else {
				m.status = "default table enabled"
			}
		case "N":
			if m.view == viewInstances {
				m.toggleSort(sortName)
			}
		case "I":
			if m.view == viewInstances {
				m.toggleSort(sortInstanceID)
			}
		case "S":
			if m.view == viewInstances {
				m.toggleSort(sortState)
			}
		case "M":
			if m.view == viewInstances {
				m.toggleSort(sortSSM)
			}
		case "P":
			if m.view == viewInstances {
				m.toggleSort(sortPrivateIP)
			}
		case "tab", "d":
			if m.view == viewInstances && m.isNarrowLayout() {
				m.detailsFocused = !m.detailsFocused
				if m.detailsFocused {
					m.status = "details view"
				} else {
					m.status = "instances view"
				}
			}
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
		case "pgup":
			if m.view == viewInstances {
				m.selected = max(0, m.selected-m.instanceTableRowLimit())
			}
		case "pgdown":
			if m.view == viewInstances && len(m.visible) > 0 {
				m.selected = min(len(m.visible)-1, m.selected+m.instanceTableRowLimit())
			}
		case "home":
			if m.view == viewInstances {
				m.selected = 0
			}
		case "end":
			if m.view == viewInstances && len(m.visible) > 0 {
				m.selected = len(m.visible) - 1
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
	case profileChangedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			if msg.auth.Mode != "" {
				m.auth = msg.auth
				m.inventory = msg.inventory
				m.identity = msg.identity
				m.regionProvider = msg.regions
				m.tunnelManager = session.NewTunnelManager(m.auth, nil)
			}
			m.status = "profile switch failed"
			return m, nil
		}
		m.auth = msg.auth
		m.inventory = msg.inventory
		m.identity = msg.identity
		m.regionProvider = msg.regions
		m.result = msg.result
		m.tunnelManager = session.NewTunnelManager(m.auth, nil)
		m.status = fmt.Sprintf("profile switched to %s", m.auth.Profile)
		m.applySearch("")
	case regionChangedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			if msg.auth.Mode != "" {
				m.auth = msg.auth
				m.inventory = msg.inventory
				m.identity = msg.identity
				m.regionProvider = msg.regions
				m.tunnelManager = session.NewTunnelManager(m.auth, nil)
			}
			m.status = "region switch failed"
			return m, nil
		}
		m.auth = msg.auth
		m.inventory = msg.inventory
		m.identity = msg.identity
		m.regionProvider = msg.regions
		m.result = msg.result
		m.tunnelManager = session.NewTunnelManager(m.auth, nil)
		m.status = fmt.Sprintf("region switched to %s", m.auth.Region)
		m.applySearch("")
	case regionsLoadedMsg:
		if msg.key != m.regionCacheKey() {
			if msg.err == nil {
				m.regionCache[msg.key] = normalizeRegionOptions(msg.regions, msg.key.Region)
			}
			return m, nil
		}
		m.regionLoading = false
		if msg.err != nil {
			m.regionLoadError = formatRegionLoadError(msg.err, m.auth)
			return m, nil
		}
		regions := normalizeRegionOptions(msg.regions, m.auth.Region)
		m.regionCache[msg.key] = regions
		m.regionOptions = regions
		m.regionSelected = optionIndex(m.regionOptions, m.auth.Region)
		if !m.regionInputDirty {
			m.regionInput = selectedRegionInput(m.regionOptions, m.regionSelected, m.auth.Region)
		}
		m.regionLoadError = ""
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	if m.searchActive {
		b.WriteString(m.renderFilterBar())
		b.WriteString("\n")
	}

	switch {
	case m.view == viewHealth:
		b.WriteString(m.renderHealth())
	case m.view == viewTunnels:
		b.WriteString(m.renderTunnels())
	case m.loading:
		b.WriteString("Loading instances...\n")
	case m.err != nil:
		b.WriteString(m.renderError())
	case len(m.visible) == 0:
		b.WriteString("No instances found.\n")
	default:
		b.WriteString(m.renderInstances())
	}
	if m.tunnelModal {
		b.WriteString("\n")
		b.WriteString(m.renderTunnelModal())
	}
	if m.profileModal {
		b.WriteString("\n")
		b.WriteString(m.renderProfileModal())
	}
	if m.regionModal {
		b.WriteString("\n")
		b.WriteString(m.renderRegionModal())
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
		if msg.String() == "s" {
			return m.applyNextTunnelPreset()
		}
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

func (m Model) applyNextTunnelPreset() (tea.Model, tea.Cmd) {
	if len(tunnelPresets) == 0 {
		return m, nil
	}
	m.tunnelPreset = (m.tunnelPreset + 1) % len(tunnelPresets)
	preset := tunnelPresets[m.tunnelPreset]
	m.localPort = preset.LocalPort
	m.remotePort = preset.RemotePort
	m.status = fmt.Sprintf("tunnel preset: %s", preset.Name)
	return m, nil
}

func (m Model) updateProfileModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.profileModal = false
		m.profileInput = ""
		return m, nil
	case tea.KeyEnter:
		profile := strings.TrimSpace(m.profileInput)
		if profile == "" {
			m.status = "profile name is required"
			return m, nil
		}
		if profile == m.auth.Profile {
			m.profileModal = false
			m.profileInput = ""
			m.status = fmt.Sprintf("profile unchanged: %s", profile)
			return m, nil
		}
		m.profileModal = false
		m.profileInput = ""
		m.loading = true
		m.err = nil
		m.status = fmt.Sprintf("switching profile to %s", profile)
		return m, m.switchProfileCmd(profile)
	case tea.KeyUp:
		if m.profileSelected > 0 {
			m.profileSelected--
			m.profileInput = m.profileOptions[m.profileSelected]
		}
	case tea.KeyDown:
		if m.profileSelected < len(m.profileOptions)-1 {
			m.profileSelected++
			m.profileInput = m.profileOptions[m.profileSelected]
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.profileInput != "" {
			runes := []rune(m.profileInput)
			m.profileInput = string(runes[:len(runes)-1])
		}
	case tea.KeyRunes:
		m.profileInput += string(msg.Runes)
	}
	return m, nil
}

func (m Model) openProfileModal() (tea.Model, tea.Cmd) {
	if m.auth.Mode == domain.AuthModeEnvActive {
		m.status = "profile switch ignored while env credentials are active"
		return m, nil
	}
	if m.tunnelManager != nil && m.tunnelManager.HasActive() {
		m.status = "profile switch blocked while tunnels are active"
		return m, nil
	}
	if m.factory == nil {
		m.status = "profile switch unavailable"
		return m, nil
	}
	m.profileInput = m.auth.Profile
	m.profileSelected = profileIndex(m.profileOptions, m.auth.Profile)
	m.profileModal = true
	m.status = ""
	return m, nil
}

func (m Model) updateRegionModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.regionModal = false
		m.regionInput = ""
		return m, nil
	case tea.KeyEnter:
		region := strings.TrimSpace(m.regionInput)
		if region == "" {
			m.status = "region is required"
			return m, nil
		}
		if region == m.auth.Region {
			m.regionModal = false
			m.regionInput = ""
			m.status = fmt.Sprintf("region unchanged: %s", region)
			return m, nil
		}
		m.regionModal = false
		m.regionInput = ""
		m.loading = true
		m.err = nil
		m.status = fmt.Sprintf("switching region to %s", region)
		return m, m.switchRegionCmd(region)
	case tea.KeyUp:
		if len(m.regionOptions) > 0 && m.regionSelected > 0 {
			m.regionSelected--
			m.regionInput = m.regionOptions[m.regionSelected]
		}
	case tea.KeyDown:
		if len(m.regionOptions) > 0 && m.regionSelected < len(m.regionOptions)-1 {
			m.regionSelected++
			m.regionInput = m.regionOptions[m.regionSelected]
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if m.regionInput != "" {
			runes := []rune(m.regionInput)
			m.regionInput = string(runes[:len(runes)-1])
		}
		m.regionInputDirty = true
	case tea.KeyRunes:
		m.regionInput += string(msg.Runes)
		m.regionInputDirty = true
	}
	return m, nil
}

func (m Model) openRegionModal() (tea.Model, tea.Cmd) {
	if m.tunnelManager != nil && m.tunnelManager.HasActive() {
		m.status = "region switch blocked while tunnels are active"
		return m, nil
	}
	if m.factory == nil {
		m.status = "region switch unavailable"
		return m, nil
	}
	m.regionInput = m.auth.Region
	m.regionInputDirty = false
	m.regionLoadError = ""
	key := m.regionCacheKey()
	if cached, ok := m.regionCache[key]; ok {
		m.regionOptions = cached
		m.regionSelected = optionIndex(m.regionOptions, m.auth.Region)
		m.regionLoading = false
	} else {
		m.regionOptions = normalizeRegionOptions(nil, m.auth.Region)
		m.regionSelected = optionIndex(m.regionOptions, m.auth.Region)
		if m.regionProvider == nil {
			m.regionLoading = false
			m.regionLoadError = "dynamic region provider is not configured; type a region manually"
		} else {
			m.regionLoading = true
		}
	}
	m.regionModal = true
	m.status = ""
	if m.regionLoading {
		return m, m.loadRegionsCmd(key)
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

func (m Model) switchProfileCmd(profile string) tea.Cmd {
	return func() tea.Msg {
		if m.factory == nil {
			return profileChangedMsg{err: errors.New("profile switch unavailable")}
		}
		auth, inventory, identity, regions, err := m.factory(context.Background(), domain.AuthContext{
			Mode:    domain.AuthModeProfileActive,
			Profile: profile,
			Region:  m.auth.Region,
		})
		if err != nil {
			return profileChangedMsg{err: err}
		}
		result, err := app.ListInstances(context.Background(), auth, inventory, identity, app.ListFilters{})
		return profileChangedMsg{
			auth:      auth,
			inventory: inventory,
			identity:  identity,
			regions:   regions,
			result:    result,
			err:       err,
		}
	}
}

func (m Model) switchRegionCmd(region string) tea.Cmd {
	return func() tea.Msg {
		if m.factory == nil {
			return regionChangedMsg{err: errors.New("region switch unavailable")}
		}
		auth, inventory, identity, regions, err := m.factory(context.Background(), domain.AuthContext{
			Mode:    m.auth.Mode,
			Profile: m.auth.Profile,
			Region:  region,
		})
		if err != nil {
			return regionChangedMsg{err: err}
		}
		result, err := app.ListInstances(context.Background(), auth, inventory, identity, app.ListFilters{})
		return regionChangedMsg{
			auth:      auth,
			inventory: inventory,
			identity:  identity,
			regions:   regions,
			result:    result,
			err:       err,
		}
	}
}

func (m Model) loadRegionsCmd(key regionCacheKey) tea.Cmd {
	return func() tea.Msg {
		if m.regionProvider == nil {
			return regionsLoadedMsg{key: key, err: errors.New("dynamic region provider is not configured")}
		}
		regions, err := m.regionProvider.ListRegions(context.Background())
		return regionsLoadedMsg{key: key, regions: regions, err: err}
	}
}

func (m Model) renderInstances() string {
	list := m.renderInstanceTable()
	details := m.renderDetails()
	if details == "" {
		return panelStyle.Render(list)
	}
	if m.isNarrowLayout() {
		if m.detailsFocused {
			return panelStyle.Render(details)
		}
		return panelStyle.Render(list)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, panelStyle.Render(list), "  ", panelStyle.Render(details))
}

func (m Model) renderHeader() string {
	var lines []string
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Center, titleStyle.Render("SeSaMe"), "  ", subtleStyle.Render("AWS SSM Session Manager")))

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
	if m.searchQuery != "" {
		signals = append(signals, renderKV("Filter", "/"+m.searchQuery))
	}
	if m.sortKey != sortNone {
		signals = append(signals, renderKV("Sort", m.sortLabel()))
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

func (m Model) renderFilterBar() string {
	query := m.searchQuery
	if query == "" {
		query = " "
	}
	return footerStyle.Render("Filter /" + query)
}

func (m Model) renderInstanceTable() string {
	var b strings.Builder
	start, end := m.instanceTableWindow()
	title := "Instances"
	if len(m.visible) > 0 {
		title = fmt.Sprintf("Instances (%d-%d of %d)", start+1, end, len(m.visible))
	}
	fmt.Fprintf(&b, "%s\n", headerStyle.Render(title))
	switch {
	case m.isCompactTable():
		fmt.Fprintf(&b, "%-2s%-16s  %-19s  %-13s\n", "", "Name", "Instance ID", "SSM")
		fmt.Fprintf(&b, "%s\n", subtleStyle.Render(strings.Repeat("─", 54)))
	case m.isMediumTable():
		fmt.Fprintf(&b, "%-2s%-18s  %-19s  %-13s  %-13s\n", "", "Name", "Instance ID", "State", "SSM")
		fmt.Fprintf(&b, "%s\n", subtleStyle.Render(strings.Repeat("─", 71)))
	case m.isWideTable():
		fmt.Fprintf(&b, "%-2s%-18s  %-19s  %-10s  %-13s  %-13s  %-15s  %-15s  %-12s\n", "", "Name", "Instance ID", "Type", "State", "SSM", "Private IP", "Public IP", "Region")
		fmt.Fprintf(&b, "%s\n", subtleStyle.Render(strings.Repeat("─", wideTableMinWidth)))
	default:
		fmt.Fprintf(&b, "%-2s%-22s  %-19s  %-13s  %-13s  %-15s\n", "", "Name", "Instance ID", "State", "SSM", "Private IP")
		fmt.Fprintf(&b, "%s\n", subtleStyle.Render(strings.Repeat("─", 91)))
	}
	for i := start; i < end; i++ {
		inst := m.visible[i]
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		row := m.renderInstanceRow(cursor, inst)
		if i == m.selected {
			row = selectedRowStyle.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) instanceTableWindow() (int, int) {
	if len(m.visible) == 0 {
		return 0, 0
	}
	limit := m.instanceTableRowLimit()
	if limit >= len(m.visible) {
		return 0, len(m.visible)
	}
	selected := min(max(0, m.selected), len(m.visible)-1)
	start := selected - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > len(m.visible) {
		start = len(m.visible) - limit
	}
	return start, start + limit
}

func (m Model) instanceTableRowLimit() int {
	if m.height <= 0 {
		return len(m.visible)
	}
	reservedRows := 10
	if m.searchActive {
		reservedRows++
	}
	if len(m.result.Warnings) > 0 || m.status != "" || m.searchQuery != "" || m.sortKey != sortNone || (m.tunnelManager != nil && len(m.tunnelManager.List()) > 0) {
		reservedRows++
	}
	if m.result.Account != "" || m.result.ARN != "" {
		reservedRows++
	}
	return max(1, m.height-reservedRows)
}

func (m Model) renderInstanceRow(cursor string, inst domain.Instance) string {
	switch {
	case m.isCompactTable():
		return fmt.Sprintf("%-2s%-16s  %-19s  %-13s",
			cursor,
			trim(inst.Name, 16),
			trim(inst.ID, 19),
			trim(string(inst.SSMStatus), 13),
		)
	case m.isMediumTable():
		return fmt.Sprintf("%-2s%-18s  %-19s  %-13s  %-13s",
			cursor,
			trim(inst.Name, 18),
			trim(inst.ID, 19),
			trim(inst.State, 13),
			trim(string(inst.SSMStatus), 13),
		)
	case m.isWideTable():
		return fmt.Sprintf("%-2s%-18s  %-19s  %-10s  %-13s  %-13s  %-15s  %-15s  %-12s",
			cursor,
			trim(inst.Name, 18),
			trim(inst.ID, 19),
			trim(inst.Type, 10),
			trim(inst.State, 13),
			trim(string(inst.SSMStatus), 13),
			trim(inst.PrivateIP, 15),
			trim(inst.PublicIP, 15),
			trim(inst.Region, 12),
		)
	default:
		return fmt.Sprintf("%-2s%-22s  %-19s  %-13s  %-13s  %-15s",
			cursor,
			trim(inst.Name, 22),
			trim(inst.ID, 19),
			trim(inst.State, 13),
			trim(string(inst.SSMStatus), 13),
			trim(inst.PrivateIP, 15),
		)
	}
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
	fmt.Fprintf(&b, "%s\n", renderKV("Created", timestampText(inst.CreatedAt)))
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
	for _, key := range keys[:min(len(keys), detailsTagLimit)] {
		fmt.Fprintf(&b, "  %s\n", renderKV(key, inst.Tags[key]))
	}
	if hidden := len(keys) - detailsTagLimit; hidden > 0 {
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
	preset := currentTunnelPresetName(m.localPort, m.remotePort)
	return fmt.Sprintf("Port forwarding\npreset: %s\n%s local port:  %s\n%s remote port: %s\nEnter start  Tab switch  s preset  Esc cancel\n", preset, localMarker, m.localPort, remoteMarker, m.remotePort)
}

func (m Model) renderProfileModal() string {
	var b strings.Builder
	b.WriteString("Profile\n")
	b.WriteString(renderOptionList(m.profileOptions, m.profileSelected))
	fmt.Fprintf(&b, "\nname: %s\n↑↓ select  type edit  Enter switch  Esc cancel\n", m.profileInput)
	return b.String()
}

func (m Model) renderRegionModal() string {
	var b strings.Builder
	b.WriteString("Region\n")
	fmt.Fprintf(&b, "current: %s\n", emptyText(m.auth.Region))
	if m.regionLoading {
		b.WriteString("regions: loading...\n")
	}
	if m.regionLoadError != "" {
		fmt.Fprintf(&b, "regions unavailable: %s\n", m.regionLoadError)
		b.WriteString("type a region manually\n")
	}
	if len(m.regionOptions) > 0 && m.regionLoadError == "" {
		b.WriteString("\n")
		b.WriteString(renderOptionList(m.regionOptions, m.regionSelected))
	}
	help := "type edit  Enter switch  Esc cancel"
	if len(m.regionOptions) > 0 && m.regionLoadError == "" {
		help = "↑↓ select  type edit  Enter switch  Esc cancel"
	}
	fmt.Fprintf(&b, "\nregion: %s\n%s\n", m.regionInput, help)
	return b.String()
}

func (m Model) renderQuitConfirm() string {
	return "Active tunnels are running.\nEnter/y stop tunnels and quit  Esc/n cancel\n"
}

func (m Model) renderHelp() string {
	sections := []string{
		renderHelpSection("Global", [][2]string{
			{"q / Ctrl+C", "quit"},
			{"?", "toggle this help"},
		}),
		renderHelpSection("Instances", [][2]string{
			{"up/down or k/j", "move selection"},
			{"/", "search"},
			{"w", "toggle wide columns"},
			{"N/I/S/M/P", "sort name/id/state/SSM/private IP"},
			{"r", "refresh inventory"},
			{"Enter", "start shell session"},
			{"f", "open port forwarding modal"},
			{"d / Tab", "show details on narrow terminals; press again to return"},
			{"t", "show tunnels"},
			{"h", "show health"},
			{"p", "choose AWS profile"},
			{"g", "choose AWS region"},
		}),
		renderHelpSection("Search", [][2]string{
			{"type", "filter visible instances"},
			{"Esc / Enter", "close search and keep filter"},
			{"Ctrl+U", "clear filter"},
		}),
		renderHelpSection("Profile Picker", [][2]string{
			{"up/down or k/j", "select profile"},
			{"type", "edit profile manually"},
			{"Enter", "switch profile"},
			{"Esc", "cancel"},
		}),
		renderHelpSection("Region Picker", [][2]string{
			{"up/down or k/j", "select loaded region"},
			{"type", "edit region manually"},
			{"Enter", "switch region"},
			{"Esc", "cancel"},
		}),
		renderHelpSection("Port Forwarding", [][2]string{
			{"Tab", "switch field"},
			{"s", "cycle port preset"},
			{"Enter", "start tunnel"},
			{"Esc", "cancel"},
		}),
		renderHelpSection("Tunnels", [][2]string{
			{"up/down or k/j", "move selection"},
			{"x", "stop selected tunnel"},
			{"c", "clear finished tunnels"},
			{"Esc", "return to instances"},
		}),
		renderHelpSection("Health", [][2]string{
			{"h", "show health checklist"},
			{"Esc", "return to instances"},
		}),
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("Help"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s\n", subtleStyle.Render("Version "+m.appVersion))
	b.WriteString(subtleStyle.Render("Close with ?, Esc, Enter, or q."))
	b.WriteString("\n\n")
	b.WriteString(strings.Join(sections, "\n"))
	b.WriteString("\n")
	return b.String()
}

func renderHelpSection(title string, rows [][2]string) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(title))
	b.WriteString("\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "  %-15s %s\n", valueStyle.Render(row[0]), row[1])
	}
	return b.String()
}

func (m Model) renderHealth() string {
	var b strings.Builder
	b.WriteString("Health\n")
	fmt.Fprintf(&b, "ok app version: %s\n", m.appVersion)
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
	if m.regionProvider == nil {
		b.WriteString("- regions: dynamic provider unavailable\n")
	} else if m.regionLoading {
		b.WriteString("- regions: loading\n")
	} else if m.regionLoadError != "" {
		fmt.Fprintf(&b, "! regions: %s\n", m.regionLoadError)
	} else if cached, ok := m.regionCache[m.regionCacheKey()]; ok {
		fmt.Fprintf(&b, "ok regions: %d loaded\n", len(cached))
	} else {
		b.WriteString("- regions: not loaded\n")
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

func (m Model) renderError() string {
	return renderError(formatTUIError(m.err, m.auth), m.contentWidth())
}

func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return min(max(20, m.width), 80)
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
			errText = " " + trim(tunnelDiagnostic(tunnel), 30)
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

func tunnelDiagnostic(tunnel domain.Tunnel) string {
	output := strings.TrimSpace(tunnel.Output)
	if output != "" {
		lines := strings.Split(output, "\n")
		return lines[len(lines)-1]
	}
	if tunnel.Err != nil {
		return tunnel.Err.Error()
	}
	return ""
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchActive = false
		return m, nil
	case tea.KeyCtrlU:
		m.searchQuery = ""
		m.applySearch("")
		return m, nil
	case tea.KeyBackspace:
		if m.searchQuery != "" {
			keepID := m.selectedInstanceID()
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.applySearch(keepID)
		}
		return m, nil
	case tea.KeyEnter:
		m.searchActive = false
		return m, nil
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
	m.applySort()
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

func (m *Model) toggleSort(key instanceSortKey) {
	keepID := m.selectedInstanceID()
	if m.sortKey == key {
		m.sortDescending = !m.sortDescending
	} else {
		m.sortKey = key
		m.sortDescending = false
	}
	m.applySearch(keepID)
	m.status = "sorted by " + m.sortLabel()
}

func (m *Model) applySort() {
	if m.sortKey == sortNone || len(m.visible) < 2 {
		return
	}
	sort.SliceStable(m.visible, func(i, j int) bool {
		cmp := compareInstances(m.visible[i], m.visible[j], m.sortKey)
		if cmp == 0 {
			cmp = strings.Compare(m.visible[i].ID, m.visible[j].ID)
		}
		if m.sortDescending {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareInstances(left, right domain.Instance, key instanceSortKey) int {
	var l, r string
	switch key {
	case sortInstanceID:
		l, r = left.ID, right.ID
	case sortState:
		l, r = left.State, right.State
	case sortSSM:
		l, r = string(left.SSMStatus), string(right.SSMStatus)
	case sortPrivateIP:
		return comparePrivateIP(left.PrivateIP, right.PrivateIP)
	default:
		l, r = left.Name, right.Name
		if l == "" {
			l = left.ID
		}
		if r == "" {
			r = right.ID
		}
	}
	return strings.Compare(strings.ToLower(l), strings.ToLower(r))
}

func comparePrivateIP(left, right string) int {
	laddr, lok := parseIP(left)
	raddr, rok := parseIP(right)
	switch {
	case lok && rok:
		return laddr.Compare(raddr)
	case lok:
		return -1
	case rok:
		return 1
	default:
		return strings.Compare(strings.ToLower(left), strings.ToLower(right))
	}
}

func parseIP(value string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	return addr, err == nil
}

func (m Model) sortLabel() string {
	direction := "asc"
	if m.sortDescending {
		direction = "desc"
	}
	return string(m.sortKey) + " " + direction
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
		return "Enter start  Tab switch field  s preset  Esc cancel"
	}
	if m.profileModal {
		return "Enter switch profile  Esc cancel"
	}
	if m.regionModal {
		return "Enter switch region  Esc cancel"
	}
	if m.view == viewTunnels {
		return "↑↓ move  x stop  c clear finished  Esc instances  q quit"
	}
	if m.view == viewHealth {
		return "Esc instances  q quit"
	}
	if m.searchActive {
		return "type filter  Ctrl+U clear  Esc/Enter close"
	}
	if m.view == viewInstances && m.isNarrowLayout() {
		wideHint := "w wide"
		if m.wideMode {
			wideHint = "w default"
		}
		if m.detailsFocused {
			return "d/Tab instances  ↑↓/Pg move  " + wideHint + "  Enter shell  f tunnel  q quit"
		}
		return "↑↓/Pg move  d/Tab details  / filter  " + wideHint + "  Enter shell  f tunnel  q quit"
	}
	return components.Footer(m.wideMode)
}

func (m Model) isNarrowLayout() bool {
	return m.width > 0 && m.width < wideDetailsMinWidth
}

func (m Model) isCompactTable() bool {
	return m.width > 0 && m.width < compactTableMinWidth
}

func (m Model) isMediumTable() bool {
	return m.width > 0 && m.width < mediumTableMinWidth && !m.isCompactTable()
}

func (m Model) isWideTable() bool {
	return m.wideMode && (m.width <= 0 || m.width >= wideTableMinWidth)
}

func (m Model) tunnels() []domain.Tunnel {
	if m.tunnelManager == nil {
		return nil
	}
	return m.tunnelManager.List()
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
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func emptyText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func normalizeAppVersion(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultAppVersion
	}
	return value
}

func lastPingText(unix int64) string {
	return timestampText(unix)
}

func timestampText(unix int64) string {
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

func renderOptionList(options []string, selected int) string {
	var b strings.Builder
	start := max(0, selected-3)
	end := min(len(options), start+7)
	if end-start < 7 {
		start = max(0, end-7)
	}
	for i := start; i < end; i++ {
		cursor := " "
		if i == selected {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", cursor, options[i])
	}
	return b.String()
}

func currentTunnelPresetName(localPort, remotePort string) string {
	for _, preset := range tunnelPresets {
		if preset.LocalPort == localPort && preset.RemotePort == remotePort {
			return preset.Name
		}
	}
	return "custom"
}

func normalizeProfileOptions(profiles []string, current string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, profile := range profiles {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			continue
		}
		if _, ok := seen[profile]; ok {
			continue
		}
		seen[profile] = struct{}{}
		normalized = append(normalized, profile)
	}
	current = strings.TrimSpace(current)
	if current != "" {
		if _, ok := seen[current]; !ok {
			normalized = append(normalized, current)
		}
	}
	if len(normalized) == 0 {
		normalized = append(normalized, "default")
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeRegionOptions(regions []string, current string) []string {
	seen := map[string]struct{}{}
	var normalized []string
	for _, region := range regions {
		region = strings.TrimSpace(region)
		if region == "" {
			continue
		}
		if _, ok := seen[region]; ok {
			continue
		}
		seen[region] = struct{}{}
		normalized = append(normalized, region)
	}
	current = strings.TrimSpace(current)
	if current != "" {
		if _, ok := seen[current]; !ok {
			normalized = append(normalized, current)
		}
	}
	sort.Strings(normalized)
	return normalized
}

func selectedRegionInput(options []string, selected int, fallback string) string {
	if selected >= 0 && selected < len(options) {
		return options[selected]
	}
	return fallback
}

func (m Model) regionCacheKey() regionCacheKey {
	return regionCacheKey{
		Mode:    m.auth.Mode,
		Profile: m.auth.Profile,
		Region:  m.auth.Region,
	}
}

func formatRegionLoadError(err error, auth domain.AuthContext) string {
	message := formatTUIError(err, auth)
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(message, "AccessDenied") || strings.Contains(lower, "denied"):
		return "ec2:DescribeRegions denied; type a region manually or ask for permission"
	case strings.Contains(lower, "credentials unavailable"):
		return "AWS credentials unavailable; fix credentials or type a region manually"
	default:
		return trim(message, 72) + "; type a region manually"
	}
}

func profileIndex(profiles []string, profile string) int {
	return optionIndex(profiles, profile)
}

func optionIndex(options []string, value string) int {
	for i, candidate := range options {
		if candidate == value {
			return i
		}
	}
	return 0
}

func renderError(message string, width int) string {
	const prefix = "Error: "
	if width <= len(prefix) {
		width = len(prefix) + 20
	}
	lines := wrapText(message, width-len(prefix))
	if len(lines) == 0 {
		lines = []string{"unknown error"}
	}

	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(&b, "%s%s\n", prefix, line)
			continue
		}
		fmt.Fprintf(&b, "%s%s\n", strings.Repeat(" ", len(prefix)), line)
	}
	return b.String()
}

func formatTUIError(err error, auth domain.AuthContext) string {
	if err == nil {
		return "unknown error"
	}

	raw := strings.TrimSpace(err.Error())
	if strings.Contains(raw, "failed to refresh cached credentials") && strings.Contains(raw, "no EC2 IMDS role found") {
		return fmt.Sprintf(
			"AWS credentials unavailable for %s. No EC2 IMDS role found. Check credentials or press p to choose another profile.",
			authSourceLabel(auth),
		)
	}
	return cleanupErrorText(raw)
}

func authSourceLabel(auth domain.AuthContext) string {
	if auth.Mode == domain.AuthModeProfileActive && auth.Profile != "" {
		return "profile " + auth.Profile
	}
	if auth.Mode == domain.AuthModeEnvActive {
		return "environment credentials"
	}
	return "current AWS configuration"
}

func cleanupErrorText(value string) string {
	value = strings.TrimSpace(value)
	replacements := []string{
		"operation error STS: GetCallerIdentity, ",
		"operation error EC2: DescribeInstances, ",
		"operation error SSM: DescribeInstanceInformation, ",
	}
	for _, old := range replacements {
		value = strings.ReplaceAll(value, old, "")
	}
	return value
}

func wrapText(value string, width int) []string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	if width < 10 {
		width = 10
	}

	lines := make([]string, 0, len(words))
	line := ""
	for _, word := range words {
		for len(word) > width {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, word[:width])
			word = word[width:]
		}
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
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
