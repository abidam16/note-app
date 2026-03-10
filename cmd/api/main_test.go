package main

import (
	"errors"
	"testing"

	"note-app/internal/infrastructure/config"
)

func TestMainSuccessAndExitBehavior(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
	}()

	cfg := config.Config{PostgresDSN: "postgres://example"}
	loadConfigFn = func() (config.Config, error) { return cfg, nil }
	depsFactoryFn = func() runtimeDeps { return runtimeDeps{} }

	exitCode := 0
	exitFn = func(code int) { exitCode = code }

	runFn = func(got config.Config, _ runtimeDeps) error {
		if got.PostgresDSN != cfg.PostgresDSN {
			t.Fatalf("unexpected cfg: %+v", got)
		}
		return nil
	}

	main()
	if exitCode != 0 {
		t.Fatalf("expected no exit on success, got %d", exitCode)
	}

	runFn = func(config.Config, runtimeDeps) error { return errors.New("run failed") }
	main()
	if exitCode != 1 {
		t.Fatalf("expected exit code 1 on run error, got %d", exitCode)
	}
}

func TestMainPanicsWhenConfigLoadFails(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
	}()

	loadConfigFn = func() (config.Config, error) { return config.Config{}, errors.New("config failed") }
	runFn = func(config.Config, runtimeDeps) error {
		t.Fatal("run should not be called when config load fails")
		return nil
	}

	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		main()
	}()

	if !didPanic {
		t.Fatal("expected panic when config load fails")
	}
}
