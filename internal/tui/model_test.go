package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sesame/internal/domain"
	"sesame/internal/health"
	"sesame/internal/session"
)

type fakeInventory struct {
	instance domain.Instance
	err      error
}

func (f fakeInventory) ListInstances(context.Context) ([]domain.Instance, []domain.Warning, error) {
	return []domain.Instance{f.instance}, []domain.Warning{}, f.err
}

func (f fakeInventory) GetInstance(context.Context, string) (domain.Instance, error) {
	return f.instance, f.err
}

type fakeIdentity struct {
	identity domain.Identity
	err      error
}

func (f fakeIdentity) GetCallerIdentity(context.Context) (domain.Identity, error) {
	return f.identity, f.err
}

func TestViewRendersInventoryAndIdentityContext(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:    model.auth,
		Region:  "eu-central-1",
		Account: "123456789012",
		ARN:     "arn:aws:sts::123456789012:assumed-role/dev/test",
		Warnings: []domain.Warning{{
			Code:    "partial",
			Message: "SSM inventory failed",
		}},
		Instances: []domain.Instance{{
			ID:        "i-123",
			Name:      "api",
			State:     "running",
			PrivateIP: "10.0.0.10",
			SSMStatus: domain.SSMStatusOnline,
		}},
	}})

	view := updated.(Model).View()
	for _, want := range []string{
		"Auth: profile-active dev",
		"Region: eu-central-1",
		"Account: 123456789012",
		"ARN: arn:aws:sts::123456789012:assumed-role/dev/test",
		"1 warning(s)",
		"api",
		"i-123",
		"online",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestViewRendersSelectedInstanceDetailsAndSortedTags(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:     model.auth,
		Region:   "eu-central-1",
		Warnings: []domain.Warning{},
		Instances: []domain.Instance{{
			ID:        "i-123",
			Name:      "api",
			State:     "running",
			Type:      "t3.micro",
			PrivateIP: "10.0.0.10",
			PublicIP:  "18.1.2.3",
			Region:    "eu-central-1",
			SSMStatus: domain.SSMStatusOnline,
			Agent: domain.AgentInfo{
				Version:          "3.2.1",
				LastPingUnixTime: 1700000000,
				PlatformType:     "Linux",
			},
			Tags: map[string]string{
				"Service":     "api",
				"Environment": "prod",
				"Name":        "api",
			},
		}},
	}})

	view := updated.(Model).View()
	for _, want := range []string{
		"Details",
		"Name: api",
		"Instance ID: i-123",
		"Type: t3.micro",
		"Private IP: 10.0.0.10",
		"Public IP: 18.1.2.3",
		"Agent version: 3.2.1",
		"Agent last ping: 2023-11-14 22:13:20 UTC",
		"Platform: Linux",
		"Tags",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected details to contain %q, got:\n%s", want, view)
		}
	}

	tagOrder := regexp.MustCompile(`(?s)Environment: prod.*Name: api.*Service: api`)
	if !tagOrder.MatchString(view) {
		t.Fatalf("expected tags sorted by key, got:\n%s", view)
	}
}

func TestInstanceTableSelectionColumnDoesNotShiftDataColumns(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.visible = []domain.Instance{{
		ID:        "i-123",
		Name:      "api",
		State:     "running",
		PrivateIP: "10.0.0.10",
		SSMStatus: domain.SSMStatusOnline,
	}}

	table := model.renderInstanceTable()
	lines := strings.Split(table, "\n")
	if len(lines) < 4 {
		t.Fatalf("expected table lines, got:\n%s", table)
	}
	header := lines[1]
	row := lines[3]
	if got, want := strings.Index(row, "i-123"), strings.Index(header, "Instance ID"); got != want {
		t.Fatalf("expected Instance ID column alignment got row index %d header index %d:\n%s", got, want, table)
	}
}

func TestDetailsLimitsRenderedTags(t *testing.T) {
	tags := map[string]string{}
	for _, key := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		tags[key] = strings.ToLower(key)
	}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:     model.auth,
		Region:   "eu-central-1",
		Warnings: []domain.Warning{},
		Instances: []domain.Instance{{
			ID:        "i-123",
			Name:      "api",
			State:     "running",
			SSMStatus: domain.SSMStatusOnline,
			Tags:      tags,
		}},
	}})

	view := updated.(Model).View()
	if !strings.Contains(view, "+ 2 more tags") {
		t.Fatalf("expected hidden tag count, got:\n%s", view)
	}
	if strings.Contains(view, "I: i") || strings.Contains(view, "J: j") {
		t.Fatalf("expected tags beyond limit to be hidden, got:\n%s", view)
	}
}

func TestViewRendersReadOnlyDependencyWarning(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: false},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:      model.auth,
		Region:    "eu-central-1",
		Warnings:  []domain.Warning{},
		Instances: []domain.Instance{},
	}})

	view := updated.(Model).View()
	if !strings.Contains(view, "READ-ONLY session-manager-plugin not found") {
		t.Fatalf("expected read-only plugin warning, got:\n%s", view)
	}
}

func TestHealthViewRendersChecklist(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: false},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:    model.auth,
		Region:  "eu-central-1",
		Account: "123456789012",
		ARN:     "arn:aws:sts::123456789012:assumed-role/dev/test",
		Warnings: []domain.Warning{{
			Code:    "ssm-warning",
			Message: "partial inventory",
		}},
		Instances: []domain.Instance{{ID: "i-123"}},
	}})
	model = updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(Model)
	view := model.View()
	for _, want := range []string{
		"Health",
		"ok aws CLI",
		"x session-manager-plugin",
		"ok auth mode: profile-active dev",
		"ok region: eu-central-1",
		"ok sts account: 123456789012",
		"ok inventory instances: 1",
		"! warnings: 1",
		"ssm-warning: partial inventory",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected health view to contain %q, got:\n%s", want, view)
		}
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.view != viewInstances {
		t.Fatalf("expected Esc to return to instances, got view %v", model.view)
	}
}

func TestInitStartsInventoryLoadAndTunnelTick(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)

	if cmd := model.Init(); cmd == nil {
		t.Fatal("expected init command")
	}
}

func TestTunnelTickSchedulesNextTick(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)

	_, cmd := model.Update(tunnelTickMsg{})
	if cmd == nil {
		t.Fatal("expected next tunnel tick command")
	}
}

func TestEnterShellBlockedInReadOnlyMode(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: false, SessionManagerPlugin: true},
	)
	model.loading = false
	model.result = domain.ListResult{
		Instances: []domain.Instance{{ID: "i-123", SSMStatus: domain.SSMStatusOnline}},
	}
	model.visible = model.result.Instances

	updated, cmd := model.startShell()
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command in read-only mode")
	}
	if !strings.Contains(got.status, "shell unavailable: aws CLI not found") {
		t.Fatalf("unexpected status: %q", got.status)
	}
}

func TestEnterShellBlockedWhenInstanceNotOnline(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.result = domain.ListResult{
		Instances: []domain.Instance{{ID: "i-123", SSMStatus: domain.SSMStatusConnectionLost}},
	}
	model.visible = model.result.Instances

	updated, cmd := model.startShell()
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command for non-online instance")
	}
	if !strings.Contains(got.status, "is not SSM online") {
		t.Fatalf("unexpected status: %q", got.status)
	}
}

func TestEnterShellStartsPreflightForOnlineInstance(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		fakeInventory{instance: target},
		fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}},
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.result = domain.ListResult{Instances: []domain.Instance{target}}
	model.visible = model.result.Instances

	updated, cmd := model.startShell()
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("expected preflight command")
	}
	if !got.shellStarting {
		t.Fatal("expected shellStarting to be true")
	}
	if !strings.Contains(got.status, "starting shell session for i-123") {
		t.Fatalf("unexpected status: %q", got.status)
	}
}

func TestShellReadyErrorUpdatesStatus(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.shellStarting = true

	updated, cmd := model.Update(shellReadyMsg{err: errors.New("preflight failed")})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no exec command on shellReady error")
	}
	if got.shellStarting {
		t.Fatal("expected shellStarting to be false")
	}
	if got.status != "preflight failed" {
		t.Fatalf("unexpected status: %q", got.status)
	}
}

func TestOpenTunnelModalForOnlineInstance(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.result = domain.ListResult{Instances: []domain.Instance{{ID: "i-123", SSMStatus: domain.SSMStatusOnline}}}
	model.visible = model.result.Instances

	updated, cmd := model.openTunnelModal()
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when opening modal")
	}
	if !got.tunnelModal {
		t.Fatal("expected tunnel modal to be open")
	}
	if !strings.Contains(got.View(), "Port forwarding") {
		t.Fatalf("expected modal in view, got:\n%s", got.View())
	}
}

func TestTunnelModalRejectsInvalidPort(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.tunnelModal = true
	model.localPort = "70000"
	model.remotePort = "22"
	model.result = domain.ListResult{Instances: []domain.Instance{{ID: "i-123", SSMStatus: domain.SSMStatusOnline}}}
	model.visible = model.result.Instances

	updated, cmd := model.startTunnel()
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command for invalid port")
	}
	if !strings.Contains(got.status, "local-port must be in range") {
		t.Fatalf("unexpected status: %q", got.status)
	}
}

func TestTunnelModalStartsTunnel(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		fakeInventory{instance: target},
		fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}},
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.tunnelModal = true
	model.localPort = "15432"
	model.remotePort = "5432"
	model.result = domain.ListResult{Instances: []domain.Instance{target}}
	model.visible = model.result.Instances
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("sleep"), func(int) error { return nil })

	updated, cmd := model.startTunnel()
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected tunnel start command")
	}
	if model.tunnelModal {
		t.Fatal("expected modal to close")
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if !strings.Contains(model.status, "tunnel running") {
		t.Fatalf("unexpected status: %q", model.status)
	}
	if !model.tunnelManager.HasActive() {
		t.Fatal("expected active tunnel")
	}
	for _, tunnel := range model.tunnelManager.List() {
		_ = model.tunnelManager.Stop(tunnel.ID)
	}
}

func TestTunnelViewRendersAndStopsTunnel(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		fakeInventory{instance: target},
		fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}},
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("sleep"), func(int) error { return nil })
	tunnel, err := model.tunnelManager.Start(context.Background(), target, 15435, 5432)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "INSTANCE ID") || !strings.Contains(view, "i-123") || !strings.Contains(view, "15435") {
		t.Fatalf("expected tunnel view, got:\n%s", view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if !strings.Contains(model.status, "stopping tunnel") {
		t.Fatalf("expected stopping status, got %q", model.status)
	}

	waitForTUITest(t, func() bool {
		for _, got := range model.tunnelManager.List() {
			if got.ID == tunnel.ID && got.State == domain.TunnelStateStopped {
				return true
			}
		}
		return false
	})
}

func TestTunnelViewClearsFinished(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("exit-ok"), func(int) error { return nil })
	if _, err := model.tunnelManager.Start(context.Background(), target, 15436, 5432); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	waitForTUITest(t, func() bool {
		tunnels := model.tunnelManager.List()
		return len(tunnels) == 1 && (tunnels[0].State == domain.TunnelStateStopped || tunnels[0].State == domain.TunnelStateFailed)
	})

	model.view = viewTunnels
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if got := model.tunnelManager.List(); len(got) != 0 {
		t.Fatalf("expected finished tunnels to be cleared, got %#v", got)
	}
}

func TestQuitWithActiveTunnelRequiresConfirmation(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("sleep"), func(int) error { return nil })
	if _, err := model.tunnelManager.Start(context.Background(), target, 15439, 5432); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected quit to wait for confirmation")
	}
	if !model.quitConfirm {
		t.Fatal("expected quit confirmation")
	}
	if !strings.Contains(model.View(), "Active tunnels are running") {
		t.Fatalf("expected confirmation in view, got:\n%s", model.View())
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected cancel to avoid quitting")
	}
	if model.quitConfirm {
		t.Fatal("expected confirmation to close after Esc")
	}
	if !model.tunnelManager.HasActive() {
		t.Fatal("expected tunnel to remain active after cancel")
	}

	model.quitConfirm = true
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected quit command after confirmation")
	}
	waitForTUITest(t, func() bool {
		return !model.tunnelManager.HasActive()
	})
}

func TestHelpOverlayOpensAndCloses(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when opening help")
	}
	if !model.helpOpen {
		t.Fatal("expected help to be open")
	}
	view := model.View()
	for _, want := range []string{"Help", "Global", "Instances", "Port forwarding modal", "Tunnels", "Close help"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected help to contain %q, got:\n%s", want, view)
		}
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command when closing help")
	}
	if model.helpOpen {
		t.Fatal("expected help to be closed")
	}
}

func TestTUITunnelHelperProcess(t *testing.T) {
	if os.Getenv("SESAME_TUI_TEST_HELPER") != "1" {
		return
	}
	switch os.Getenv("SESAME_TUI_TEST_HELPER_MODE") {
	case "sleep":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func tuiTestCommandFactory(mode string) session.CommandFactory {
	return func(context.Context, domain.Instance, int, int) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestTUITunnelHelperProcess")
		cmd.Env = append(os.Environ(),
			"SESAME_TUI_TEST_HELPER=1",
			"SESAME_TUI_TEST_HELPER_MODE="+mode,
		)
		return cmd
	}
}

func waitForTUITest(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before deadline")
}

func TestSearchFiltersVisibleInstances(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:     model.auth,
		Region:   "eu-central-1",
		Warnings: []domain.Warning{},
		Instances: []domain.Instance{
			{ID: "i-1", Name: "api", State: "running", SSMStatus: domain.SSMStatusOnline},
			{ID: "i-2", Name: "bastion", State: "running", SSMStatus: domain.SSMStatusOnline},
		},
	}})
	model = updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	if !model.searchActive {
		t.Fatal("expected search to be active")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = updated.(Model)

	if len(model.visible) != 1 || model.visible[0].ID != "i-1" {
		t.Fatalf("expected search to filter to api instance, got %#v", model.visible)
	}
	if !strings.Contains(model.View(), "Search: /ap") {
		t.Fatalf("expected search query in view, got:\n%s", model.View())
	}
}

func TestSearchEscClearsThenCloses(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:     model.auth,
		Region:   "eu-central-1",
		Warnings: []domain.Warning{},
		Instances: []domain.Instance{
			{ID: "i-1", Name: "api", State: "running", SSMStatus: domain.SSMStatusOnline},
			{ID: "i-2", Name: "bastion", State: "running", SSMStatus: domain.SSMStatusOnline},
		},
	}})
	model = updated.(Model)
	model.searchActive = true
	model.searchQuery = "api"
	model.applySearch("")

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.searchQuery != "" || len(model.visible) != 2 || !model.searchActive {
		t.Fatalf("expected first Esc to clear query and keep search active, got query=%q active=%v visible=%d", model.searchQuery, model.searchActive, len(model.visible))
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.searchActive {
		t.Fatal("expected second Esc to close search")
	}
}
