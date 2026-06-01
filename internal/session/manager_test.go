package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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

func TestTunnelManagerStopSendsInterruptBeforeKillFallback(t *testing.T) {
	dir := t.TempDir()
	readyFile := filepath.Join(dir, "ready")
	interruptFile := filepath.Join(dir, "interrupted")
	manager := NewTunnelManagerWithPortChecker(
		domain.AuthContext{Mode: domain.AuthModeEnvActive, Region: "eu-central-1"},
		testCommandFactoryWithEnv("trap-interrupt", map[string]string{
			"SESAME_TEST_READY_FILE":     readyFile,
			"SESAME_TEST_INTERRUPT_FILE": interruptFile,
		}),
		allowPort,
	)
	tunnel, err := manager.Start(context.Background(), domain.Instance{ID: "i-1"}, 15441, 5432)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	waitFor(t, func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	})

	if err := manager.Stop(tunnel.ID); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	waitFor(t, func() bool {
		_, err := os.Stat(interruptFile)
		return err == nil
	})
	waitFor(t, func() bool {
		tunnels := manager.List()
		return len(tunnels) == 1 && tunnels[0].State == domain.TunnelStateStopped
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
		return len(tunnels) == 1 &&
			tunnels[0].State == domain.TunnelStateFailed &&
			tunnels[0].Err != nil &&
			tunnels[0].Output == "helper failed\n"
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
	case "trap-interrupt":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt)
		defer signal.Stop(signals)
		readyPath := os.Getenv("SESAME_TEST_READY_FILE")
		if readyPath != "" {
			_ = os.WriteFile(readyPath, []byte("ready"), 0o600)
		}
		select {
		case <-signals:
			path := os.Getenv("SESAME_TEST_INTERRUPT_FILE")
			if path != "" {
				_ = os.WriteFile(path, []byte("interrupted"), 0o600)
			}
			os.Exit(0)
		case <-time.After(10 * time.Second):
			os.Exit(3)
		}
	case "exit-ok":
		os.Exit(0)
	case "exit-fail":
		fmt.Fprintln(os.Stderr, "helper failed")
		os.Exit(7)
	default:
		os.Exit(2)
	}
}

func testCommandFactory(mode string) CommandFactory {
	return testCommandFactoryWithEnv(mode, nil)
}

func testCommandFactoryWithEnv(mode string, extra map[string]string) CommandFactory {
	return func(context.Context, domain.Instance, int, int) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
		env := append(os.Environ(),
			"SESAME_TEST_HELPER=1",
			"SESAME_TEST_HELPER_MODE="+mode,
		)
		for key, value := range extra {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
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
