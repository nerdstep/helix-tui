package main

import (
	"reflect"
	"testing"

	"helix-tui/internal/app"
	"helix-tui/internal/configfile"
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
			path, explicit, err := parseConfigPath(tt.args)
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

	t.Setenv("APCA_API_KEY_ID", "env-key")
	t.Setenv("APCA_API_SECRET_KEY", "env-secret")
	t.Setenv("APCA_API_DATA_URL", "env-url")

	applyEnvOverrides(&cfg)
	if cfg.AlpacaAPIKey != "env-key" {
		t.Fatalf("unexpected key: %q", cfg.AlpacaAPIKey)
	}
	if cfg.AlpacaAPISecret != "env-secret" {
		t.Fatalf("unexpected secret: %q", cfg.AlpacaAPISecret)
	}
	if cfg.AlpacaDataURL != "env-url" {
		t.Fatalf("unexpected data url: %q", cfg.AlpacaDataURL)
	}
}

func TestSplitSymbols(t *testing.T) {
	got := splitSymbols("aapl, AAPL, msft ,, tsla")
	want := []string{"AAPL", "MSFT", "TSLA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSymbols mismatch: got %#v want %#v", got, want)
	}
}
