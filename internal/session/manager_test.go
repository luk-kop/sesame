package session

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"sesame/internal/domain"
)

func TestTunnelManagerStartAndDuplicate(t *testing.T) {
	manager := NewTunnelManagerWithPortChecker(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		testCommandFactory("sleep"),
		allowPort,
	)
	target := domain.Instance{ID: "i-123"}

	tunnel, err := manager.Start(context.Background(), target, 15432, 5432)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if tunnel.State != domain.TunnelStateRunning {
		t.Fatalf("expected running tunnel, got %#v", tunnel)
	}
	if !manager.HasActive() {
		t.Fatal("expected active tunnel")
	}

	_, err = manager.Start(context.Background(), target, 15432, 5432)
	if err == nil {
		t.Fatal("expected duplicate tunnel error")
	}

	if err := manager.Stop(tunnel.ID); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	waitFor(t, func() bool {
		tunnels := manager.List()
		return len(tunnels) == 1 && tunnels[0].State == domain.TunnelStateStopped
	})
}

func TestTunnelManagerClearFinished(t *testing.T) {
	manager := NewTunnelManagerWithPortChecker(
		domain.AuthContext{Mode: domain.AuthModeProfileActive, Profile: "dev", Region: "eu-central-1"},
		testCommandFactory("exit-ok"),
		allowPort,
	)

	tunnel, err := manager.Start(context.Background(), domain.Instance{ID: "i-123"}, 15433, 5432)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	waitFor(t, func() bool {
		tunnels := manager.List()
		return len(tunnels) == 1 && tunnels[0].ID == tunnel.ID && tunnels[0].State == domain.TunnelStateStopped
	})

	manager.ClearFinished()
	if got := manager.List(); len(got) != 0 {
		t.Fatalf("expected finished tunnels to be cleared, got %#v", got)
	}
}

func TestTunnelManagerStopAll(t *testing.T) {
	manager := NewTunnelManagerWithPortChecker(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		testCommandFactory("sleep"),
		allowPort,
	)
	if _, err := manager.Start(context.Background(), domain.Instance{ID: "i-1"}, 15437, 5432); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if _, err := manager.Start(context.Background(), domain.Instance{ID: "i-2"}, 15438, 5432); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	manager.StopAll()

	waitFor(t, func() bool {
		tunnels := manager.List()
		if len(tunnels) != 2 {
			return false
		}
		for _, tunnel := range tunnels {
			if tunnel.State != domain.TunnelStateStopped {
				return false
			}
		}
		return true
	})
}

func TestTunnelManagerRecordsFailedProcess(t *testing.T) {
	manager := NewTunnelManagerWithPortChecker(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		testCommandFactory("exit-fail"),
		allowPort,
	)

	_, err := manager.Start(context.Background(), domain.Instance{ID: "i-123"}, 15434, 5432)
	if err != nil {
		t.Fatalf("Start should succeed before process exits, got %v", err)
	}

	waitFor(t, func() bool {
		tunnels := manager.List()
		return len(tunnels) == 1 && tunnels[0].State == domain.TunnelStateFailed && tunnels[0].Err != nil
	})
}

func TestTunnelManagerRejectsInvalidPort(t *testing.T) {
	manager := NewTunnelManagerWithPortChecker(domain.AuthContext{Region: "eu-central-1"}, testCommandFactory("exit-ok"), allowPort)
	_, err := manager.Start(context.Background(), domain.Instance{ID: "i-123"}, 0, 5432)
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("SESAME_TEST_HELPER") != "1" {
		return
	}
	switch os.Getenv("SESAME_TEST_HELPER_MODE") {
	case "sleep":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "exit-ok":
		os.Exit(0)
	case "exit-fail":
		os.Exit(7)
	default:
		os.Exit(2)
	}
}

func testCommandFactory(mode string) CommandFactory {
	return func(context.Context, domain.Instance, int, int) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
		cmd.Env = append(os.Environ(),
			"SESAME_TEST_HELPER=1",
			"SESAME_TEST_HELPER_MODE="+mode,
		)
		return cmd
	}
}

func allowPort(int) error {
	return nil
}

func waitFor(t *testing.T, check func() bool) {
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
