package runtime

import (
	"fmt"
	"os"
	"strings"

	"helix-tui/internal/app"
	"helix-tui/internal/configfile"
	"helix-tui/internal/domain"
)

func loadConfig(path string, required bool) (app.Config, error) {
	cfg := configfile.Default()
	if err := configfile.Load(path, &cfg, required); err != nil {
		return app.Config{}, fmt.Errorf("failed to load config: %w", err)
	}
	ApplyEnvOverrides(&cfg)
	if err := normalizeAndValidateConfig(&cfg); err != nil {
		return app.Config{}, err
	}
	return cfg.ToAppConfig(), nil
}

func ApplyEnvOverrides(cfg *configfile.Config) {
	if v := strings.TrimSpace(os.Getenv("APCA_API_KEY_ID")); v != "" {
		cfg.Alpaca.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("APCA_API_SECRET_KEY")); v != "" {
		cfg.Alpaca.APISecret = v
	}
	if v := strings.TrimSpace(os.Getenv("APCA_API_DATA_URL")); v != "" {
		cfg.Alpaca.DataURL = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
		cfg.Agent.LLM.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("HELIX_LLM_API_KEY")); v != "" {
		cfg.Agent.LLM.APIKey = v
	}
}

func normalizeAndValidateConfig(cfg *configfile.Config) error {
	normalizeConfig(cfg)
	return validateConfig(*cfg)
}

func normalizeConfig(cfg *configfile.Config) {
	cfg.Normalize()
	cfg.Logging.Mode = normalizedLogMode(cfg.Logging.Mode)
	cfg.Logging.Level = normalizedLogLevel(cfg.Logging.Level)
	switch mode := domain.Mode(cfg.Mode); mode {
	case domain.ModeManual, domain.ModeAssist, domain.ModeAuto:
	default:
		cfg.Mode = string(domain.ModeManual)
	}
}

func validateConfig(cfg configfile.Config) error {
	if _, err := logFileOpenFlags(cfg.Logging.Mode); err != nil {
		return err
	}
	if _, err := logLevelFromString(cfg.Logging.Level); err != nil {
		return err
	}
	if cfg.Agent.MinGainPct < 0 {
		return fmt.Errorf("agent.min_gain_pct must be >= 0")
	}
	switch cfg.Compliance.AccountType {
	case "", "auto", "margin", "cash":
	default:
		return fmt.Errorf("compliance.account_type must be one of auto|margin|cash")
	}
	if cfg.Compliance.MaxDayTrades5D < 0 {
		return fmt.Errorf("compliance.max_day_trades_5d must be >= 0")
	}
	if cfg.Compliance.MinEquityForPDT < 0 {
		return fmt.Errorf("compliance.min_equity_for_pdt must be >= 0")
	}
	if cfg.Compliance.SettlementDays <= 0 {
		return fmt.Errorf("compliance.settlement_days must be > 0")
	}
	switch cfg.Agent.LLM.ContextLog {
	case "", "off", "summary", "full":
	default:
		return fmt.Errorf("agent.llm.context_log must be one of off|summary|full")
	}
	return nil
}
