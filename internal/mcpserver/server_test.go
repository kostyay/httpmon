package mcpserver

import (
	"context"
	"testing"
	"time"
)

// --- New ---

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("default_addr", func(t *testing.T) {
		s := New(Config{})
		if s.Addr() != DefaultAddr {
			t.Errorf("addr = %q, want %q", s.Addr(), DefaultAddr)
		}
	})

	t.Run("custom_addr", func(t *testing.T) {
		s := New(Config{Addr: "0.0.0.0:8080"})
		if s.Addr() != "0.0.0.0:8080" {
			t.Errorf("addr = %q", s.Addr())
		}
	})
}

// --- Start / Stop lifecycle ---

func TestStartStop(t *testing.T) {
	t.Parallel()
	s := New(Config{Store: &mockStore{}, Addr: "127.0.0.1:0"})

	if s.Running() {
		t.Fatal("should not be running before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Running() {
		t.Fatal("should be running after Start")
	}

	s.Stop()
	if s.Running() {
		t.Fatal("should not be running after Stop")
	}
}

func TestDoubleStart(t *testing.T) {
	t.Parallel()
	s := New(Config{Store: &mockStore{}, Addr: "127.0.0.1:0"})

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	if err := s.Start(context.Background()); err == nil {
		t.Fatal("expected error on double start")
	}
}

func TestDoubleStopIdempotent(t *testing.T) {
	t.Parallel()
	s := New(Config{Store: &mockStore{}, Addr: "127.0.0.1:0"})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	s.Stop() // should not panic
}

func TestContextCancelStops(t *testing.T) {
	t.Parallel()
	s := New(Config{Store: &mockStore{}, Addr: "127.0.0.1:0"})
	ctx, cancel := context.WithCancel(context.Background())

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()

	// Give goroutine time to process cancel.
	time.Sleep(100 * time.Millisecond)
	if s.Running() {
		t.Error("should stop after context cancel")
	}
}

// --- Port ---

func TestPort(t *testing.T) {
	t.Parallel()
	s := New(Config{Store: &mockStore{}, Addr: "127.0.0.1:0"})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	if port := s.Port(); port <= 0 {
		t.Errorf("port = %d, want > 0", port)
	}
}

func TestPortNotStarted(t *testing.T) {
	t.Parallel()
	s := New(Config{Addr: "bad"})
	if p := s.Port(); p != 0 {
		t.Errorf("port = %d, want 0 for invalid addr", p)
	}
}

// --- registerTools ---

func TestRegisterToolsReadOnly(t *testing.T) {
	t.Parallel()
	// Only Store set -- sim/script tools should not be registered.
	// Verify indirectly: registerTools doesn't panic.
	s := New(Config{Store: &mockStore{}, Addr: "127.0.0.1:0"})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
}

func TestRegisterToolsWithSimAndScripts(t *testing.T) {
	t.Parallel()
	s := New(Config{
		Store:    &mockStore{},
		Proxy:    &mockProxy{addr: "127.0.0.1:8888"},
		Throttle: &mockThrottle{},
		Scripts:  &mockScripts{dir: t.TempDir()},
		Addr:     "127.0.0.1:0",
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
}
