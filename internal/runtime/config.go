package runtime

import (
	"fmt"
	"os"
	"strings"

	"helix-tui/internal/app"
	"helix-tui/internal/configfile"
	"helix-tui/internal/symbols"
)

func loadConfig(args []string) (app.Config, string, error) {
	cfg := app.DefaultConfig()
	path, explicit, err := ParseConfigPath(args)
	if err != nil {
		return cfg, "", fmt.Errorf("invalid config flag: %w", err)
	}
	if err := configfile.Load(path, &cfg, explicit); err != nil {
		return cfg, "", fmt.Errorf("failed to load config: %w", err)
	}
	ApplyEnvOverrides(&cfg)
	return cfg, path, nil
}

func ParseConfigPath(args []string) (string, bool, error) {
	path := configfile.DefaultPath
	explicit := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-config" || arg == "--config":
			if i+1 >= len(args) {
				return "", false, fmt.Errorf("%s requires a path value", arg)
			}
			path = strings.TrimSpace(args[i+1])
			explicit = true
			i++
		case strings.HasPrefix(arg, "-config="):
			path = strings.TrimSpace(strings.TrimPrefix(arg, "-config="))
			explicit = true
		case strings.HasPrefix(arg, "--config="):
			path = strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			explicit = true
		}
	}

	if path == "" {
		return "", false, fmt.Errorf("config path cannot be empty")
	}
	return path, explicit, nil
}

func ApplyEnvOverrides(cfg *app.Config) {
	if v := strings.TrimSpace(os.Getenv("APCA_API_KEY_ID")); v != "" {
		cfg.AlpacaAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("APCA_API_SECRET_KEY")); v != "" {
		cfg.AlpacaAPISecret = v
	}
	if v := strings.TrimSpace(os.Getenv("APCA_API_DATA_URL")); v != "" {
		cfg.AlpacaDataURL = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
		cfg.LLMAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("HELIX_LLM_API_KEY")); v != "" {
		cfg.LLMAPIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("HELIX_LOG_FILE")); v != "" {
		cfg.LogFile = v
	}
	if v := strings.TrimSpace(os.Getenv("HELIX_LOG_MODE")); v != "" {
		cfg.LogMode = strings.ToLower(v)
	}
	if v := strings.TrimSpace(os.Getenv("HELIX_DB_PATH")); v != "" {
		cfg.DatabasePath = v
	}
}

func SplitSymbols(raw string) []string {
	return symbols.Normalize(strings.Split(raw, ","))
}
