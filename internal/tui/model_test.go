package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sesame/internal/app"
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

type fakeRegionProvider struct {
	regions []string
	err     error
	calls   int
}

func (f *fakeRegionProvider) ListRegions(context.Context) ([]string, error) {
	f.calls++
	return append([]string(nil), f.regions...), f.err
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
		"SeSaMe",
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
			CreatedAt: 1600000000,
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
		"Created: 2020-09-13 12:26:40 UTC",
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

func TestNarrowLayoutTogglesDetailsView(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	model = updated.(Model)
	updated, _ = model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:   model.auth,
		Region: "eu-central-1",
		Instances: []domain.Instance{{
			ID:        "i-123",
			Name:      "api",
			State:     "running",
			Type:      "t3.micro",
			PrivateIP: "10.0.0.10",
			SSMStatus: domain.SSMStatusOnline,
		}},
	}})
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "Instances") || strings.Contains(view, "Details") {
		t.Fatalf("expected narrow layout to start with instance list only, got:\n%s", view)
	}
	if !strings.Contains(view, "d/Tab details") {
		t.Fatalf("expected footer to advertise details toggle, got:\n%s", view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	view = model.View()
	if !strings.Contains(view, "Details") || strings.Contains(view, "Instances") {
		t.Fatalf("expected narrow layout to show details only after toggle, got:\n%s", view)
	}
	if !strings.Contains(view, "d/Tab instances") {
		t.Fatalf("expected footer to advertise instances toggle, got:\n%s", view)
	}
}

func TestWideLayoutKeepsListAndDetailsTogether(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	model = updated.(Model)
	updated, _ = model.Update(inventoryLoadedMsg{result: domain.ListResult{
		Auth:   model.auth,
		Region: "eu-central-1",
		Instances: []domain.Instance{{
			ID:        "i-123",
			Name:      "api",
			State:     "running",
			Type:      "t3.micro",
			PrivateIP: "10.0.0.10",
			SSMStatus: domain.SSMStatusOnline,
		}},
	}})
	model = updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "Instances") || !strings.Contains(view, "Details") {
		t.Fatalf("expected wide layout to keep list and details together, got:\n%s", view)
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

func TestInstanceTableHidesPrivateIPOnMediumWidth(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.width = 90
	model.visible = []domain.Instance{{
		ID:        "i-123",
		Name:      "api",
		State:     "running",
		PrivateIP: "10.0.0.10",
		SSMStatus: domain.SSMStatusOnline,
	}}

	table := model.renderInstanceTable()
	if strings.Contains(table, "Private IP") || strings.Contains(table, "10.0.0.10") {
		t.Fatalf("expected medium table to hide private IP, got:\n%s", table)
	}
	for _, want := range []string{"Name", "Instance ID", "State", "SSM", "running"} {
		if !strings.Contains(table, want) {
			t.Fatalf("expected medium table to contain %q, got:\n%s", want, table)
		}
	}
}

func TestInstanceTableUsesCompactColumnsOnNarrowWidth(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.width = 60
	model.visible = []domain.Instance{{
		ID:        "i-123",
		Name:      "api",
		State:     "running",
		PrivateIP: "10.0.0.10",
		SSMStatus: domain.SSMStatusOnline,
	}}

	table := model.renderInstanceTable()
	for _, notWant := range []string{"State", "Private IP", "running", "10.0.0.10"} {
		if strings.Contains(table, notWant) {
			t.Fatalf("expected compact table to hide %q, got:\n%s", notWant, table)
		}
	}
	for _, want := range []string{"Name", "Instance ID", "SSM", "online"} {
		if !strings.Contains(table, want) {
			t.Fatalf("expected compact table to contain %q, got:\n%s", want, table)
		}
	}
}

func TestWideModeAddsOperationalColumnsWhenTerminalIsWideEnough(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.width = 140
	model.visible = []domain.Instance{{
		ID:        "i-123",
		Name:      "api",
		State:     "running",
		Type:      "t3.micro",
		PrivateIP: "10.0.0.10",
		PublicIP:  "18.1.2.3",
		Region:    "eu-central-1",
		SSMStatus: domain.SSMStatusOnline,
	}}

	table := model.renderInstanceTable()
	for _, notWant := range []string{"Type", "Public IP", "Region", "t3.micro", "18.1.2.3"} {
		if strings.Contains(table, notWant) {
			t.Fatalf("expected default table to hide %q, got:\n%s", notWant, table)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = updated.(Model)
	table = model.renderInstanceTable()
	for _, want := range []string{"Type", "Public IP", "Region", "t3.micro", "18.1.2.3", "eu-central-1"} {
		if !strings.Contains(table, want) {
			t.Fatalf("expected wide table to contain %q, got:\n%s", want, table)
		}
	}
	if !strings.Contains(model.footer(), "w default") {
		t.Fatalf("expected footer to advertise default toggle, got %q", model.footer())
	}
}

func TestInstanceTableShowsLongEC2State(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.visible = []domain.Instance{{
		ID:        "i-123",
		Name:      "api",
		State:     "shutting-down",
		PrivateIP: "10.0.0.10",
		SSMStatus: domain.SSMStatusOnline,
	}}

	table := model.renderInstanceTable()
	if !strings.Contains(table, "shutting-down") {
		t.Fatalf("expected table to show full EC2 state, got:\n%s", table)
	}
}

func TestWideModeFallsBackOnNarrowTerminal(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.width = 90
	model.visible = []domain.Instance{{
		ID:        "i-123",
		Name:      "api",
		State:     "running",
		Type:      "t3.micro",
		PrivateIP: "10.0.0.10",
		PublicIP:  "18.1.2.3",
		Region:    "eu-central-1",
		SSMStatus: domain.SSMStatusOnline,
	}}
	model.wideMode = true

	table := model.renderInstanceTable()
	for _, notWant := range []string{"Type", "Public IP", "t3.micro", "18.1.2.3"} {
		if strings.Contains(table, notWant) {
			t.Fatalf("expected narrow terminal to ignore wide columns %q, got:\n%s", notWant, table)
		}
	}
}

func TestInstanceTableLimitsRowsToTerminalHeight(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.height = 12
	model.visible = testInstances(20)

	table := model.renderInstanceTable()
	if strings.Contains(table, "api-02") {
		t.Fatalf("expected table to render only visible window rows, got:\n%s", table)
	}
	for _, want := range []string{"Instances (1-2 of 20)", "api-00", "api-01"} {
		if !strings.Contains(table, want) {
			t.Fatalf("expected table to contain %q, got:\n%s", want, table)
		}
	}
}

func TestInstanceTableWindowFollowsSelection(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.height = 12
	model.visible = testInstances(20)
	model.selected = 10

	table := model.renderInstanceTable()
	for _, want := range []string{"Instances (10-11 of 20)", "> api-10"} {
		if !strings.Contains(table, want) {
			t.Fatalf("expected selected instance in visible window with %q, got:\n%s", want, table)
		}
	}
	if strings.Contains(table, "api-00") || strings.Contains(table, "api-19") {
		t.Fatalf("expected table to omit rows outside selected window, got:\n%s", table)
	}
}

func TestPageDownMovesSelectionByVisibleRows(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.view = viewInstances
	model.height = 12
	model.visible = testInstances(20)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)

	if model.selected != 2 {
		t.Fatalf("expected PgDown to move by visible row count, got %d", model.selected)
	}
}

func TestSortShortcutsOrderVisibleInstancesAndToggleDirection(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.view = viewInstances
	model.result = domain.ListResult{Instances: []domain.Instance{
		{ID: "i-2", Name: "worker", State: "stopped", PrivateIP: "10.0.0.20", SSMStatus: domain.SSMStatusConnectionLost},
		{ID: "i-1", Name: "api", State: "running", PrivateIP: "10.0.0.10", SSMStatus: domain.SSMStatusOnline},
	}}
	model.visible = append([]domain.Instance(nil), model.result.Instances...)
	model.selected = 1

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	model = updated.(Model)
	if got := []string{model.visible[0].ID, model.visible[1].ID}; got[0] != "i-1" || got[1] != "i-2" {
		t.Fatalf("expected private IP ascending sort, got %#v", got)
	}
	if model.selected != 0 {
		t.Fatalf("expected selected instance to be preserved after sort, got selected=%d", model.selected)
	}
	if !strings.Contains(model.View(), "Sort: private IP asc") {
		t.Fatalf("expected sort in header, got:\n%s", model.View())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	model = updated.(Model)
	if got := []string{model.visible[0].ID, model.visible[1].ID}; got[0] != "i-2" || got[1] != "i-1" {
		t.Fatalf("expected private IP descending sort, got %#v", got)
	}
	if !strings.Contains(model.View(), "Sort: private IP desc") {
		t.Fatalf("expected descending sort in header, got:\n%s", model.View())
	}
}

func TestDetailsRendersAllTags(t *testing.T) {
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
	for _, want := range []string{"A: a", "H: h", "I: i", "J: j"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected details to contain tag %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "more tags") {
		t.Fatalf("expected details to render all tags without hidden count, got:\n%s", view)
	}
}

func testInstances(count int) []domain.Instance {
	instances := make([]domain.Instance, count)
	for i := range instances {
		instances[i] = domain.Instance{
			ID:        fmt.Sprintf("i-%03d", i),
			Name:      fmt.Sprintf("api-%02d", i),
			State:     "running",
			PrivateIP: fmt.Sprintf("10.0.0.%d", i+10),
			SSMStatus: domain.SSMStatusOnline,
		}
	}
	return instances
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

func TestViewRendersFriendlyWrappedCredentialError(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "default", Region: "eu-west-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.width = 64
	model.err = errors.New("get caller identity: operation error STS: GetCallerIdentity, get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found, operation error ec2imds: GetMetadata, request canceled")

	view := model.View()
	for _, want := range []string{
		"Error: AWS credentials unavailable for profile default.",
		"No EC2",
		"IMDS role found.",
		"press p to choose",
		"another profile.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected error view to contain %q, got:\n%s", want, view)
		}
	}
	for _, notWant := range []string{
		"operation error STS",
		"failed to refresh cached credentials",
	} {
		if strings.Contains(view, notWant) {
			t.Fatalf("expected error view to hide raw text %q, got:\n%s", notWant, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "Error: ") || strings.HasPrefix(line, "       ") {
			if len(line) > model.width {
				t.Fatalf("expected wrapped error line to fit width %d, got %d: %q\n%s", model.width, len(line), line, view)
			}
		}
	}
}

func TestErrorWidthIsCappedInWideTerminals(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "default", Region: "eu-west-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.loading = false
	model.width = 160
	model.err = errors.New("get caller identity: operation error STS: GetCallerIdentity, get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found")

	view := model.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "Error: ") || strings.HasPrefix(line, "       ") {
			if len(line) > 80 {
				t.Fatalf("expected wide terminal error line to be capped at 80 chars, got %d: %q\n%s", len(line), line, view)
			}
		}
	}
}

func TestProfileSwitchReloadsInventoryFromErrorView(t *testing.T) {
	target := domain.Instance{ID: "i-123", Name: "api", State: "running", SSMStatus: domain.SSMStatusOnline}
	factoryCalls := 0
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "default", Region: "eu-west-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(_ context.Context, auth domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			factoryCalls++
			if auth.Profile != "dev" {
				t.Fatalf("expected requested profile dev, got %q", auth.Profile)
			}
			auth.Region = "eu-west-1"
			return auth, fakeInventory{instance: target}, fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}}, nil, nil
		},
		[]string{"default", "dev", "prod"},
	)
	model.loading = false
	model.err = errors.New("credentials failed")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected profile modal to open without command")
	}
	if !model.profileModal || model.profileInput != "default" {
		t.Fatalf("expected profile modal with current profile, got modal=%v input=%q", model.profileModal, model.profileInput)
	}
	if !strings.Contains(model.View(), "> default") || !strings.Contains(model.View(), "  dev") {
		t.Fatalf("expected profile picker list, got:\n%s", model.View())
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command while selecting profile")
	}
	if model.profileInput != "dev" {
		t.Fatalf("expected selected profile dev, got %q", model.profileInput)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected profile switch command")
	}
	if !model.loading || model.err != nil {
		t.Fatalf("expected loading state with cleared error, loading=%v err=%v", model.loading, model.err)
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if factoryCalls != 1 {
		t.Fatalf("expected one factory call, got %d", factoryCalls)
	}
	if model.auth.Profile != "dev" || model.err != nil || model.loading {
		t.Fatalf("expected switched profile without error, auth=%#v loading=%v err=%v", model.auth, model.loading, model.err)
	}
	if len(model.visible) != 1 || model.visible[0].ID != "i-123" {
		t.Fatalf("expected reloaded inventory, got %#v", model.visible)
	}
	if !strings.Contains(model.status, "profile switched to dev") {
		t.Fatalf("unexpected status: %q", model.status)
	}
}

func TestProfileSwitchIgnoredForEnvActive(t *testing.T) {
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-west-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called for env-active")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"default", "dev"},
	)
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = updated.(Model)
	if cmd != nil || model.profileModal {
		t.Fatalf("expected no modal or command, modal=%v cmd=%v", model.profileModal, cmd)
	}
	if !strings.Contains(model.status, "env credentials are active") {
		t.Fatalf("unexpected status: %q", model.status)
	}
}

func TestProfileSwitchBlockedWithActiveTunnel(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-west-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called while tunnels are active")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"dev", "prod"},
	)
	model.loading = false
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("sleep"), func(int) error { return nil })
	tunnel, err := model.tunnelManager.Start(context.Background(), target, 15443, 22)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = model.tunnelManager.Stop(tunnel.ID)
	}()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = updated.(Model)
	if cmd != nil || model.profileModal {
		t.Fatalf("expected no modal or command, modal=%v cmd=%v", model.profileModal, cmd)
	}
	if !strings.Contains(model.status, "profile switch blocked while tunnels are active") {
		t.Fatalf("unexpected status: %q", model.status)
	}
}

func TestProfileSwitchInventoryErrorKeepsNewAuthContext(t *testing.T) {
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "default", Region: "eu-west-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(_ context.Context, auth domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			return auth, fakeInventory{err: errors.New("inventory denied")}, fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}}, nil, nil
		},
		[]string{"default", "dev"},
	)
	model.loading = false

	msg := model.switchProfileCmd("dev")()
	updated, _ := model.Update(msg)
	model = updated.(Model)
	if model.auth.Profile != "dev" {
		t.Fatalf("expected new profile in auth context, got %#v", model.auth)
	}
	if model.err == nil || !strings.Contains(model.err.Error(), "inventory denied") {
		t.Fatalf("expected inventory error, got %v", model.err)
	}
	if model.status != "profile switch failed" {
		t.Fatalf("expected concise profile status, got %q", model.status)
	}
	if !strings.Contains(model.View(), "Auth: profile-active dev") {
		t.Fatalf("expected header to show new profile, got:\n%s", model.View())
	}
}

func TestRegionSwitchReloadsInventoryFromErrorView(t *testing.T) {
	target := domain.Instance{ID: "i-456", Name: "worker", State: "running", Region: "eu-west-2", SSMStatus: domain.SSMStatusOnline}
	regionProvider := &fakeRegionProvider{regions: []string{"eu-west-2", "eu-west-1"}}
	factoryCalls := 0
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-west-1"},
		nil,
		nil,
		regionProvider,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(_ context.Context, auth domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			factoryCalls++
			if auth.Profile != "dev" {
				t.Fatalf("expected profile dev, got %q", auth.Profile)
			}
			if auth.Region != "eu-west-2" {
				t.Fatalf("expected requested region eu-west-2, got %q", auth.Region)
			}
			return auth, fakeInventory{instance: target}, fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}}, regionProvider, nil
		},
		[]string{"dev"},
	)
	model.loading = false
	model.err = errors.New("region failed")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected dynamic region load command")
	}
	if !model.regionModal || model.regionInput != "eu-west-1" {
		t.Fatalf("expected region modal with current region, got modal=%v input=%q", model.regionModal, model.regionInput)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if !strings.Contains(model.View(), "> eu-west-1") || !strings.Contains(model.View(), "  eu-west-2") {
		t.Fatalf("expected region picker list, got:\n%s", model.View())
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command while selecting region")
	}
	if model.regionInput != "eu-west-2" {
		t.Fatalf("expected selected region eu-west-2, got %q", model.regionInput)
	}
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected region switch command")
	}
	if !model.loading || model.err != nil {
		t.Fatalf("expected loading state with cleared error, loading=%v err=%v", model.loading, model.err)
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if factoryCalls != 1 {
		t.Fatalf("expected one factory call, got %d", factoryCalls)
	}
	if model.auth.Region != "eu-west-2" || model.err != nil || model.loading {
		t.Fatalf("expected switched region without error, auth=%#v loading=%v err=%v", model.auth, model.loading, model.err)
	}
	if len(model.visible) != 1 || model.visible[0].Region != "eu-west-2" {
		t.Fatalf("expected reloaded region inventory, got %#v", model.visible)
	}
	if !strings.Contains(model.status, "region switched to eu-west-2") {
		t.Fatalf("unexpected status: %q", model.status)
	}
}

func TestRegionModalLoadsRegionsLazilyAndCachesSuccess(t *testing.T) {
	regions := &fakeRegionProvider{regions: []string{"eu-west-2", "eu-central-1"}}
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		regions,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called when opening region modal")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"dev"},
	)
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected async region load command")
	}
	if !model.regionModal || !model.regionLoading {
		t.Fatalf("expected loading region modal, modal=%v loading=%v", model.regionModal, model.regionLoading)
	}
	if !strings.Contains(model.View(), "regions: loading") {
		t.Fatalf("expected loading message, got:\n%s", model.View())
	}

	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if regions.calls != 1 {
		t.Fatalf("expected one region provider call, got %d", regions.calls)
	}
	if model.regionLoading || model.regionLoadError != "" {
		t.Fatalf("expected loaded regions without error, loading=%v err=%q", model.regionLoading, model.regionLoadError)
	}
	view := model.View()
	if !strings.Contains(view, "> eu-central-1") || !strings.Contains(view, "  eu-west-2") {
		t.Fatalf("expected sorted loaded region list, got:\n%s", view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("expected cached regions to avoid another load command")
	}
	if regions.calls != 1 {
		t.Fatalf("expected cached regions to keep one provider call, got %d", regions.calls)
	}
}

func TestRegionModalDoesNotCacheErrorsAndShowsHealthDiagnostic(t *testing.T) {
	regions := &fakeRegionProvider{err: errors.New("AccessDenied: ec2:DescribeRegions")}
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		regions,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called when opening region modal")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"dev"},
	)
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected region load command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if regions.calls != 1 {
		t.Fatalf("expected one failed load, got %d", regions.calls)
	}
	view := model.View()
	for _, want := range []string{"regions unavailable:", "ec2:DescribeRegions denied", "ask for permission", "type a region manually"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected region error view to contain %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "↑↓ select") {
		t.Fatalf("expected no select hint when region list failed, got:\n%s", view)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(Model)
	healthView := model.View()
	if !strings.Contains(healthView, "! regions:") || !strings.Contains(healthView, "ec2:DescribeRegions denied") {
		t.Fatalf("expected health view region diagnostic, got:\n%s", healthView)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected failed region load not to be cached")
	}
}

func TestRegionModalCredentialErrorIsConcise(t *testing.T) {
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "default", Region: "eu-west-1"},
		nil,
		nil,
		&fakeRegionProvider{err: errors.New("get caller identity: operation error STS: GetCallerIdentity, get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found")},
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called when loading region suggestions")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"default"},
	)
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected region load command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	view := model.View()
	if !strings.Contains(view, "regions unavailable: AWS credentials unavailable; fix credentials or type a region manually") {
		t.Fatalf("expected concise region credential error, got:\n%s", view)
	}
	for _, notWant := range []string{"No EC2 IMDS role found. Check credentials", "choose another pr", "failed to refresh cached credentials"} {
		if strings.Contains(view, notWant) {
			t.Fatalf("expected region modal to avoid verbose raw credential text %q, got:\n%s", notWant, view)
		}
	}
}

func TestRegionLoadDoesNotOverwriteDirtyManualInput(t *testing.T) {
	regions := &fakeRegionProvider{regions: []string{"eu-west-1", "eu-west-2"}}
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		regions,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called when loading regions")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"dev"},
	)
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected region load command")
	}
	model.regionInput = ""
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("us-gov-west-1")})
	model = updated.(Model)
	if !model.regionInputDirty {
		t.Fatal("expected manual input to mark region input dirty")
	}

	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.regionInput != "us-gov-west-1" {
		t.Fatalf("expected loaded regions not to overwrite manual input, got %q", model.regionInput)
	}
}

func TestStaleRegionLoadDoesNotOverwriteCurrentModalState(t *testing.T) {
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		nil,
		nil,
		&fakeRegionProvider{regions: []string{"eu-central-1"}},
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		nil,
		[]string{"dev"},
	)
	model.loading = false
	model.regionModal = true
	model.regionLoading = true
	staleKey := regionCacheKey{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-west-1"}

	updated, _ := model.Update(regionsLoadedMsg{
		key:     staleKey,
		regions: []string{"eu-west-1", "eu-west-2"},
		err:     errors.New("stale denied"),
	})
	model = updated.(Model)
	if !model.regionLoading || model.regionLoadError != "" {
		t.Fatalf("expected stale error not to alter current load state, loading=%v err=%q", model.regionLoading, model.regionLoadError)
	}
	if _, ok := model.regionCache[staleKey]; ok {
		t.Fatal("expected stale error not to be cached")
	}

	updated, _ = model.Update(regionsLoadedMsg{
		key:     staleKey,
		regions: []string{"eu-west-1", "eu-west-2"},
	})
	model = updated.(Model)
	if !model.regionLoading || model.regionLoadError != "" {
		t.Fatalf("expected stale success not to alter current load state, loading=%v err=%q", model.regionLoading, model.regionLoadError)
	}
	if cached := model.regionCache[staleKey]; len(cached) != 2 || cached[0] != "eu-west-1" || cached[1] != "eu-west-2" {
		t.Fatalf("expected stale success to populate cache only, got %#v", cached)
	}
}

func TestRegionSwitchInventoryErrorKeepsNewAuthContext(t *testing.T) {
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-west-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(_ context.Context, auth domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			return auth, fakeInventory{err: errors.New("region denied")}, fakeIdentity{identity: domain.Identity{Account: "123", ARN: "arn"}}, nil, nil
		},
		[]string{"dev"},
	)
	model.loading = false

	msg := model.switchRegionCmd("eu-west-2")()
	updated, _ := model.Update(msg)
	model = updated.(Model)
	if model.auth.Region != "eu-west-2" {
		t.Fatalf("expected new region in auth context, got %#v", model.auth)
	}
	if model.err == nil || !strings.Contains(model.err.Error(), "region denied") {
		t.Fatalf("expected inventory error, got %v", model.err)
	}
	if model.status != "region switch failed" {
		t.Fatalf("expected concise region status, got %q", model.status)
	}
	if !strings.Contains(model.View(), "Region: eu-west-2") {
		t.Fatalf("expected header to show new region, got:\n%s", model.View())
	}
}

func TestRegionSwitchCredentialErrorDoesNotLeakRawSDKErrorIntoStatus(t *testing.T) {
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "default", Region: "eu-central-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(_ context.Context, auth domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			return auth, fakeInventory{err: errors.New("get caller identity: operation error STS: GetCallerIdentity, get identity: get credentials: failed to refresh cached credentials, no EC2 IMDS role found")}, fakeIdentity{}, nil, nil
		},
		[]string{"default"},
	)
	model.loading = false

	msg := model.switchRegionCmd("eu-central-1")()
	updated, _ := model.Update(msg)
	model = updated.(Model)

	view := model.View()
	if strings.Contains(view, "Status: region switch failed: get caller identity") {
		t.Fatalf("expected status to omit raw SDK error, got:\n%s", view)
	}
	if !strings.Contains(view, "Status: region switch failed") {
		t.Fatalf("expected concise status, got:\n%s", view)
	}
	if !strings.Contains(view, "Error: AWS credentials unavailable for profile default.") {
		t.Fatalf("expected friendly error body, got:\n%s", view)
	}
}

func TestRegionSwitchBlockedWithActiveTunnel(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModelWithProviderFactory(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-west-1"},
		nil,
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
		func(context.Context, domain.AuthContext) (domain.AuthContext, app.InventoryProvider, app.IdentityProvider, app.RegionProvider, error) {
			t.Fatal("factory should not be called while tunnels are active")
			return domain.AuthContext{}, nil, nil, nil, nil
		},
		[]string{"dev"},
	)
	model.loading = false
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("sleep"), func(int) error { return nil })
	tunnel, err := model.tunnelManager.Start(context.Background(), target, 15440, 22)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer func() {
		_ = model.tunnelManager.Stop(tunnel.ID)
	}()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(Model)
	if cmd != nil || model.regionModal {
		t.Fatalf("expected no modal or command, modal=%v cmd=%v", model.regionModal, cmd)
	}
	if !strings.Contains(model.status, "region switch blocked while tunnels are active") {
		t.Fatalf("unexpected status: %q", model.status)
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
		"ok app version: dev revision=unknown build_date=unknown",
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
	if !strings.Contains(got.View(), "preset: SSH") {
		t.Fatalf("expected default SSH preset in modal, got:\n%s", got.View())
	}
}

func TestTunnelModalCyclesPortPresets(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.tunnelModal = true

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("expected no command while cycling presets")
	}
	if got.localPort != "15432" || got.remotePort != "5432" {
		t.Fatalf("expected PostgreSQL preset ports, got local=%q remote=%q", got.localPort, got.remotePort)
	}
	if !strings.Contains(got.View(), "preset: PostgreSQL") {
		t.Fatalf("expected PostgreSQL preset in modal, got:\n%s", got.View())
	}
	if !strings.Contains(got.footer(), "s preset") {
		t.Fatalf("expected footer to advertise presets, got %q", got.footer())
	}
}

func TestTunnelModalShowsCustomPresetForManualPorts(t *testing.T) {
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.tunnelModal = true
	model.localPort = "19999"
	model.remotePort = "9999"

	view := model.View()
	if !strings.Contains(view, "preset: custom") {
		t.Fatalf("expected custom preset in modal, got:\n%s", view)
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

func TestTunnelViewShowsProcessOutputOnFailure(t *testing.T) {
	target := domain.Instance{ID: "i-123", SSMStatus: domain.SSMStatusOnline}
	model := NewModel(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		nil,
		nil,
		health.DependencyStatus{AWSCLI: true, SessionManagerPlugin: true},
	)
	model.tunnelManager = session.NewTunnelManagerWithPortChecker(model.auth, tuiTestCommandFactory("exit-fail"), func(int) error { return nil })
	if _, err := model.tunnelManager.Start(context.Background(), target, 15442, 5432); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	waitForTUITest(t, func() bool {
		tunnels := model.tunnelManager.List()
		return len(tunnels) == 1 && tunnels[0].State == domain.TunnelStateFailed
	})

	model.view = viewTunnels
	view := model.View()
	if !strings.Contains(view, "failed") || !strings.Contains(view, "tui helper failed") {
		t.Fatalf("expected failed tunnel output in view, got:\n%s", view)
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
	for _, want := range []string{"Help", "Version dev revision=unknown build_date=unknown", "Global", "Instances", "d / Tab", "Profile Picker", "Region Picker", "Port Forwarding", "Tunnels", "Close with"} {
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
	case "exit-fail":
		_, _ = os.Stderr.WriteString("tui helper failed\n")
		os.Exit(7)
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
	if !strings.Contains(model.View(), "Filter /ap") {
		t.Fatalf("expected filter bar in view, got:\n%s", model.View())
	}
}

func TestSearchEscClosesAndKeepsFilter(t *testing.T) {
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
	if model.searchQuery != "api" || len(model.visible) != 1 || model.searchActive {
		t.Fatalf("expected Esc to close search and keep filter, got query=%q active=%v visible=%d", model.searchQuery, model.searchActive, len(model.visible))
	}
	if !strings.Contains(model.View(), "Filter: /api") {
		t.Fatalf("expected inactive filter in header, got:\n%s", model.View())
	}
}

func TestSearchCtrlUClearFilter(t *testing.T) {
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

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	model = updated.(Model)
	if model.searchQuery != "" || len(model.visible) != 2 || !model.searchActive {
		t.Fatalf("expected Ctrl+U to clear filter and keep search active, got query=%q active=%v visible=%d", model.searchQuery, model.searchActive, len(model.visible))
	}
}
