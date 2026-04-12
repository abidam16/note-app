package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"note-app/internal/infrastructure/config"
)

func TestMainSuccessAndExitBehavior(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	originalArgs := os.Args
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
		os.Args = originalArgs
	}()

	cfg := config.Config{PostgresDSN: "postgres://example"}
	loadConfigFn = func(envFile string) (config.Config, error) {
		if envFile != ".env.local" {
			t.Fatalf("unexpected env file: %s", envFile)
		}
		return cfg, nil
	}
	depsFactoryFn = func() runtimeDeps { return runtimeDeps{} }
	os.Args = []string{"api", "-env-file", ".env.local"}

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

func TestMainExitsWhenConfigLoadFails(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	originalArgs := os.Args
	originalStderr := stderrWriter
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
		os.Args = originalArgs
		stderrWriter = originalStderr
	}()

	loadConfigFn = func(string) (config.Config, error) { return config.Config{}, errors.New("config failed") }
	runFn = func(config.Config, runtimeDeps) error {
		t.Fatal("run should not be called when config load fails")
		return nil
	}
	os.Args = []string{"api"}
	var stderr bytes.Buffer
	stderrWriter = &stderr

	exitCode := 0
	exitFn = func(code int) { exitCode = code }

	main()

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "config failed") {
		t.Fatalf("expected config error to be written to stderr, got %q", stderr.String())
	}
}

func TestMainExitsWhenEnvFlagParseFails(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	originalArgs := os.Args
	originalStderr := stderrWriter
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
		os.Args = originalArgs
		stderrWriter = originalStderr
	}()

	loadConfigFn = func(string) (config.Config, error) {
		t.Fatal("loadConfig should not be called when flag parsing fails")
		return config.Config{}, nil
	}
	runFn = func(config.Config, runtimeDeps) error {
		t.Fatal("run should not be called when flag parsing fails")
		return nil
	}
	os.Args = []string{"api", "-bad-flag"}
	var stderr bytes.Buffer
	stderrWriter = &stderr

	exitCode := 0
	exitFn = func(code int) { exitCode = code }

	main()

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("expected flag parse error to be written to stderr, got %q", stderr.String())
	}
}

func TestParseEnvFile(t *testing.T) {
	envFile, err := parseEnvFile(nil)
	if err != nil {
		t.Fatalf("parseEnvFile() default error = %v", err)
	}
	if envFile != ".env" {
		t.Fatalf("expected default env file, got %q", envFile)
	}

	envFile, err = parseEnvFile([]string{"-env-file", ".env.test"})
	if err != nil {
		t.Fatalf("parseEnvFile() custom error = %v", err)
	}
	if envFile != ".env.test" {
		t.Fatalf("expected custom env file, got %q", envFile)
	}

	if _, err := parseEnvFile([]string{"-bad-flag"}); err == nil {
		t.Fatal("expected parseEnvFile flag error")
	}
}
