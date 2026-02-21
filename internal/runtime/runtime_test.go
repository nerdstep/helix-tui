package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helix-tui/internal/configfile"
	"helix-tui/internal/domain"
)

func TestParseRunOptions_DefaultPath(t *testing.T) {
	opts, err := parseRunOptions([]string{"-headless"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseRunOptions failed: %v", err)
	}
	if !opts.headless {
		t.Fatalf("expected headless option")
	}
	if opts.configPath != configfile.DefaultPath {
		t.Fatalf("unexpected config path: %q", opts.configPath)
	}
	if opts.configExplicit {
		t.Fatalf("did not expect explicit config flag")
	}
}

func TestParseRunOptions_ExplicitConfig(t *testing.T) {
	tests := [][]string{
		{"-config=custom.toml"},
		{"-config", "custom.toml"},
		{"--config=custom.toml"},
	}
	for _, args := range tests {
		opts, err := parseRunOptions(args, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("parseRunOptions(%v) failed: %v", args, err)
		}
		if opts.configPath != "custom.toml" {
			t.Fatalf("unexpected config path for %v: %q", args, opts.configPath)
		}
		if !opts.configExplicit {
			t.Fatalf("expected explicit config for %v", args)
		}
	}
}

func TestParseRunOptions_DefaultStderr(t *testing.T) {
	if _, err := parseRunOptions([]string{"-headless"}, nil); err != nil {
		t.Fatalf("parseRunOptions with nil stderr failed: %v", err)
	}
}

func TestParseRunOptions_RejectsLegacyFlags(t *testing.T) {
	_, err := parseRunOptions([]string{"-mode=auto"}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected error for unsupported flag")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRunOptions_EmptyConfigPath(t *testing.T) {
	_, err := parseRunOptions([]string{"-config="}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected empty config path error")
	}
	if !strings.Contains(err.Error(), "config path cannot be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	cfg := configfile.Default()
	cfg.Alpaca.APIKey = "from-config-key"
	cfg.Alpaca.APISecret = "from-config-secret"
	cfg.Alpaca.DataURL = "from-config-url"
	cfg.Agent.LLM.APIKey = "from-config-llm"

	t.Setenv("APCA_API_KEY_ID", "env-key")
	t.Setenv("APCA_API_SECRET_KEY", "env-secret")
	t.Setenv("APCA_API_DATA_URL", "env-url")
	t.Setenv("OPENAI_API_KEY", "env-llm-key")

	ApplyEnvOverrides(&cfg)
	if cfg.Alpaca.APIKey != "env-key" {
		t.Fatalf("unexpected key: %q", cfg.Alpaca.APIKey)
	}
	if cfg.Alpaca.APISecret != "env-secret" {
		t.Fatalf("unexpected secret: %q", cfg.Alpaca.APISecret)
	}
	if cfg.Alpaca.DataURL != "env-url" {
		t.Fatalf("unexpected data url: %q", cfg.Alpaca.DataURL)
	}
	if cfg.Agent.LLM.APIKey != "env-llm-key" {
		t.Fatalf("unexpected llm key: %q", cfg.Agent.LLM.APIKey)
	}
}

func TestNormalizeAndValidateConfig(t *testing.T) {
	cfg := configfile.Default()
	cfg.Mode = " AUTO "
	cfg.Agent.Type = " LLM "
	cfg.Agent.Watchlist = []string{"tsla", " TSLA ", "nvda"}
	cfg.Agent.LLM.ContextLog = " SUMMARY "
	cfg.Logging.Mode = " APPEND "
	cfg.Logging.Level = " DEBUG "
	cfg.Compliance.AccountType = " MARGIN "

	if err := normalizeAndValidateConfig(&cfg); err != nil {
		t.Fatalf("normalizeAndValidateConfig failed: %v", err)
	}
	if cfg.Mode != "auto" {
		t.Fatalf("unexpected normalized mode: %q", cfg.Mode)
	}
	if cfg.Agent.Type != "llm" {
		t.Fatalf("unexpected normalized agent type: %q", cfg.Agent.Type)
	}
	if cfg.Logging.Mode != "append" {
		t.Fatalf("unexpected normalized log mode: %q", cfg.Logging.Mode)
	}
	if cfg.Logging.Level != "debug" {
		t.Fatalf("unexpected normalized log level: %q", cfg.Logging.Level)
	}
	if got := strings.Join(cfg.Agent.Watchlist, ","); got != "TSLA,NVDA" {
		t.Fatalf("unexpected watchlist: %q", got)
	}
	if cfg.Agent.LLM.ContextLog != "summary" {
		t.Fatalf("unexpected context log mode: %q", cfg.Agent.LLM.ContextLog)
	}
	if cfg.Compliance.AccountType != "margin" {
		t.Fatalf("unexpected compliance account type: %q", cfg.Compliance.AccountType)
	}
}

func TestNormalizeAndValidateConfig_InvalidLogMode(t *testing.T) {
	cfg := configfile.Default()
	cfg.Logging.Mode = "rotate"
	if err := normalizeAndValidateConfig(&cfg); err == nil {
		t.Fatalf("expected invalid log mode error")
	}
}

func TestNormalizeAndValidateConfig_InvalidLogLevel(t *testing.T) {
	cfg := configfile.Default()
	cfg.Logging.Level = "verbose"
	if err := normalizeAndValidateConfig(&cfg); err == nil {
		t.Fatalf("expected invalid log level error")
	}
}

func TestNormalizeAndValidateConfig_InvalidMinGainPct(t *testing.T) {
	cfg := configfile.Default()
	cfg.Agent.MinGainPct = -1
	if err := normalizeAndValidateConfig(&cfg); err == nil {
		t.Fatalf("expected invalid min gain pct error")
	}
}

func TestNormalizeAndValidateConfig_InvalidLLMContextLog(t *testing.T) {
	cfg := configfile.Default()
	cfg.Agent.LLM.ContextLog = "verbose"
	if err := normalizeAndValidateConfig(&cfg); err == nil {
		t.Fatalf("expected invalid llm context log error")
	}
}

func TestNormalizeAndValidateConfig_InvalidComplianceAccountType(t *testing.T) {
	cfg := configfile.Default()
	cfg.Compliance.AccountType = "prime"
	if err := normalizeAndValidateConfig(&cfg); err == nil {
		t.Fatalf("expected invalid compliance account type error")
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
	err = Run(ctx, []string{"-headless"}, stderr)
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

	badConfigPath := filepath.Join(t.TempDir(), "bad.toml")
	if writeErr := os.WriteFile(badConfigPath, []byte("broker = \"unsupported\"\n"), 0o644); writeErr != nil {
		t.Fatalf("WriteFile failed: %v", writeErr)
	}
	err = Run(ctx, []string{"-headless", "-config=" + badConfigPath}, stderr)
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
