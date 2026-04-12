package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"note-app/internal/application"
	"note-app/internal/infrastructure/config"
)

func TestParseFlags(t *testing.T) {
	input, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags() default error = %v", err)
	}
	if input.EnvFile != ".env" || input.WorkspaceID != "" || input.DryRun || input.BatchSize != 500 {
		t.Fatalf("unexpected defaults: %+v", input)
	}

	input, err = parseFlags([]string{"-env-file", ".env.test", "-workspace-id", "workspace-1", "-dry-run", "-batch-size", "25"})
	if err != nil {
		t.Fatalf("parseFlags() custom error = %v", err)
	}
	if input.EnvFile != ".env.test" || input.WorkspaceID != "workspace-1" || !input.DryRun || input.BatchSize != 25 {
		t.Fatalf("unexpected parsed flags: %+v", input)
	}

	if _, err := parseFlags([]string{"-batch-size", "0"}); err == nil || !strings.Contains(err.Error(), "batch-size") {
		t.Fatalf("expected batch-size validation error, got %v", err)
	}
	if _, err := parseFlags([]string{"-batch-size", "2001"}); err == nil || !strings.Contains(err.Error(), "batch-size") {
		t.Fatalf("expected batch-size validation error, got %v", err)
	}
}

func TestMainSuccessAndExitBehavior(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	originalArgs := os.Args
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
		os.Args = originalArgs
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	cfg := config.Config{PostgresDSN: "postgres://example"}
	loadConfigFn = func(envFile string) (config.Config, error) {
		if envFile != ".env.local" {
			t.Fatalf("unexpected env file: %s", envFile)
		}
		return cfg, nil
	}
	depsFactoryFn = func() runtimeDeps { return runtimeDeps{} }
	os.Args = []string{"notification-reconcile", "-env-file", ".env.local", "-dry-run", "-batch-size", "25"}

	exitCode := 0
	exitFn = func(code int) { exitCode = code }

	var stdout bytes.Buffer
	stdoutWriter = &stdout

	runFn = func(got config.Config, _ runtimeDeps, input reconcileInput) (application.NotificationReconciliationSummary, error) {
		if got.PostgresDSN != cfg.PostgresDSN {
			t.Fatalf("unexpected cfg: %+v", got)
		}
		if !input.DryRun || input.WorkspaceID != "" || input.BatchSize != 25 {
			t.Fatalf("unexpected input: %+v", input)
		}
		return application.NotificationReconciliationSummary{Status: "ok", DryRun: true, BatchSize: 25}, nil
	}

	main()

	if exitCode != 0 {
		t.Fatalf("expected no exit on success, got %d", exitCode)
	}
	var got application.NotificationReconciliationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal stdout summary: %v", err)
	}
	if got.Status != "ok" || !got.DryRun || got.BatchSize != 25 {
		t.Fatalf("unexpected stdout summary: %+v", got)
	}
	if stdout.Len() == 0 {
		t.Fatal("expected summary to be written to stdout")
	}

	runFn = func(config.Config, runtimeDeps, reconcileInput) (application.NotificationReconciliationSummary, error) {
		return application.NotificationReconciliationSummary{}, errors.New("run failed")
	}
	stdout.Reset()
	main()
	if exitCode != 1 {
		t.Fatalf("expected exit code 1 on run error, got %d", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should remain empty on run failure, got %q", stdout.String())
	}
}

func TestMainExitsWhenConfigLoadFails(t *testing.T) {
	originalLoad := loadConfigFn
	originalRun := runFn
	originalDepsFactory := depsFactoryFn
	originalExit := exitFn
	originalArgs := os.Args
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		loadConfigFn = originalLoad
		runFn = originalRun
		depsFactoryFn = originalDepsFactory
		exitFn = originalExit
		os.Args = originalArgs
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	loadConfigFn = func(string) (config.Config, error) {
		return config.Config{}, errors.New("config failed")
	}
	runFn = func(config.Config, runtimeDeps, reconcileInput) (application.NotificationReconciliationSummary, error) {
		t.Fatal("run should not be called when config load fails")
		return application.NotificationReconciliationSummary{}, nil
	}
	os.Args = []string{"notification-reconcile"}
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
