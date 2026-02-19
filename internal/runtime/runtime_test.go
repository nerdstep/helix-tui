package runtime

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"helix-tui/internal/app"
	"helix-tui/internal/configfile"
	"helix-tui/internal/domain"
)

func TestParseConfigPath(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantPath     string
		wantExplicit bool
		wantErr      bool
	}{
		{
			name:         "default path",
			args:         nil,
			wantPath:     configfile.DefaultPath,
			wantExplicit: false,
		},
		{
			name:         "short flag with value",
			args:         []string{"-config", "custom.toml"},
			wantPath:     "custom.toml",
			wantExplicit: true,
		},
		{
			name:         "long flag with equals",
			args:         []string{"--config=custom.toml"},
			wantPath:     "custom.toml",
			wantExplicit: true,
		},
		{
			name:    "missing value",
			args:    []string{"-config"},
			wantErr: true,
		},
		{
			name:    "empty value",
			args:    []string{"-config="},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, explicit, err := ParseConfigPath(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if path != tt.wantPath {
				t.Fatalf("path mismatch: got %q want %q", path, tt.wantPath)
			}
			if explicit != tt.wantExplicit {
				t.Fatalf("explicit mismatch: got %v want %v", explicit, tt.wantExplicit)
			}
		})
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	cfg := app.DefaultConfig()
	cfg.AlpacaAPIKey = "from-config-key"
	cfg.AlpacaAPISecret = "from-config-secret"
	cfg.AlpacaDataURL = "from-config-url"
	cfg.LLMAPIKey = "from-config-llm"

	t.Setenv("APCA_API_KEY_ID", "env-key")
	t.Setenv("APCA_API_SECRET_KEY", "env-secret")
	t.Setenv("APCA_API_DATA_URL", "env-url")
	t.Setenv("OPENAI_API_KEY", "env-llm-key")
	t.Setenv("HELIX_LOG_FILE", "logs/from-env.log")
	t.Setenv("HELIX_LOG_MODE", "truncate")

	ApplyEnvOverrides(&cfg)
	if cfg.AlpacaAPIKey != "env-key" {
		t.Fatalf("unexpected key: %q", cfg.AlpacaAPIKey)
	}
	if cfg.AlpacaAPISecret != "env-secret" {
		t.Fatalf("unexpected secret: %q", cfg.AlpacaAPISecret)
	}
	if cfg.AlpacaDataURL != "env-url" {
		t.Fatalf("unexpected data url: %q", cfg.AlpacaDataURL)
	}
	if cfg.LLMAPIKey != "env-llm-key" {
		t.Fatalf("unexpected llm key: %q", cfg.LLMAPIKey)
	}
	if cfg.LogFile != "logs/from-env.log" {
		t.Fatalf("unexpected log file: %q", cfg.LogFile)
	}
	if cfg.LogMode != "truncate" {
		t.Fatalf("unexpected log mode: %q", cfg.LogMode)
	}
}

func TestSplitSymbols(t *testing.T) {
	got := SplitSymbols("aapl, AAPL, msft ,, tsla")
	want := []string{"AAPL", "MSFT", "TSLA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitSymbols mismatch: got %#v want %#v", got, want)
	}
}

func TestParseRunOptions(t *testing.T) {
	cfg := app.DefaultConfig()
	opts, err := parseRunOptions(
		[]string{
			"-allow=aapl,msft",
			"-watchlist=tsla,nvda",
			"-mode=ASSIST",
			"-headless",
			"-agent-type=llm",
			"-sync-timeout=12s",
			"-order-timeout=14s",
			"-log-file=logs/helix.log",
			"-log-mode=truncate",
			"-llm-model=gpt-4.1-mini",
			"-llm-key=test-key",
		},
		cfg,
		configfile.DefaultPath,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("parseRunOptions failed: %v", err)
	}
	if !opts.headless {
		t.Fatalf("expected headless option")
	}
	if opts.cfg.Mode != domain.ModeAssist {
		t.Fatalf("unexpected mode: %q", opts.cfg.Mode)
	}
	if opts.cfg.AgentType != "llm" {
		t.Fatalf("unexpected agent type: %q", opts.cfg.AgentType)
	}
	if opts.cfg.LLMModel != "gpt-4.1-mini" || opts.cfg.LLMAPIKey != "test-key" {
		t.Fatalf("unexpected llm config: model=%q key=%q", opts.cfg.LLMModel, opts.cfg.LLMAPIKey)
	}
	if opts.cfg.SyncTimeout != 12*time.Second || opts.cfg.OrderTimeout != 14*time.Second {
		t.Fatalf("unexpected timeout config: sync=%s order=%s", opts.cfg.SyncTimeout, opts.cfg.OrderTimeout)
	}
	if opts.cfg.LogFile != "logs/helix.log" {
		t.Fatalf("unexpected log file: %q", opts.cfg.LogFile)
	}
	if opts.cfg.LogMode != "truncate" {
		t.Fatalf("unexpected log mode: %q", opts.cfg.LogMode)
	}
	wantAllow := []string{"AAPL", "MSFT"}
	if !reflect.DeepEqual(opts.cfg.AllowSymbols, wantAllow) {
		t.Fatalf("allow symbols mismatch: got %#v want %#v", opts.cfg.AllowSymbols, wantAllow)
	}
	wantWatchlist := []string{"TSLA", "NVDA"}
	if !reflect.DeepEqual(opts.cfg.Watchlist, wantWatchlist) {
		t.Fatalf("watchlist mismatch: got %#v want %#v", opts.cfg.Watchlist, wantWatchlist)
	}
}

func TestParseRunOptions_DefaultStderr(t *testing.T) {
	cfg := app.DefaultConfig()
	if _, err := parseRunOptions([]string{"-mode=auto"}, cfg, configfile.DefaultPath, nil); err != nil {
		t.Fatalf("parseRunOptions with nil stderr failed: %v", err)
	}
}

func TestParseRunOptions_InvalidLogMode(t *testing.T) {
	cfg := app.DefaultConfig()
	_, err := parseRunOptions([]string{"-log-mode=rotate"}, cfg, configfile.DefaultPath, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected invalid log mode error")
	}
	if !strings.Contains(err.Error(), "invalid log mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunHeadlessStopsOnCanceledContext(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	RunHeadless(ctx, headlessEngineStub{})
	_ = w.Close()

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if out == "" {
		t.Fatalf("expected headless output")
	}
}

func TestRunHeadlessManual(t *testing.T) {
	oldStdout := os.Stdout
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stderr := &bytes.Buffer{}
	err = Run(ctx, []string{"-headless", "-broker=paper", "-mode=manual"}, stderr)
	_ = w.Close()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestRunErrors(t *testing.T) {
	stderr := &bytes.Buffer{}
	ctx := context.Background()

	err := Run(ctx, []string{"-does-not-exist"}, stderr)
	if err == nil {
		t.Fatalf("expected parse error")
	}

	err = Run(ctx, []string{"-headless", "-broker=unsupported"}, stderr)
	if err == nil || !strings.Contains(err.Error(), "unsupported broker") {
		t.Fatalf("expected unsupported broker error, got %v", err)
	}

	err = Run(ctx, []string{"-config=does-not-exist.toml"}, stderr)
	if err == nil || !strings.Contains(err.Error(), "failed to load config") {
		t.Fatalf("expected config load error, got %v", err)
	}
}

type headlessEngineStub struct{}

func (headlessEngineStub) Snapshot() domain.Snapshot {
	return domain.Snapshot{
		Account: domain.Account{Cash: 1, Equity: 1},
	}
}
