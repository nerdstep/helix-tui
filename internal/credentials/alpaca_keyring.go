package credentials

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	DefaultService = "helix-tui"
	DefaultUser    = "alpaca-paper"
	keyIDField     = "alpaca_api_key_id"
	secretField    = "alpaca_api_secret_key"
)

type KeyringConfig struct {
	Enabled bool
	Save    bool
	Service string
	User    string
}

func ResolveAlpacaCredentials(apiKey, secret string, cfg KeyringConfig) (string, string, string, error) {
	key := strings.TrimSpace(apiKey)
	sec := strings.TrimSpace(secret)
	source := "flags/env"

	if !cfg.Enabled {
		if key == "" || sec == "" {
			return "", "", "", fmt.Errorf("alpaca API key and secret are required")
		}
		return key, sec, source, nil
	}

	service := serviceName(cfg.Service)
	user := userName(cfg.User)

	if key == "" || sec == "" {
		storedKey, storedSecret, err := LoadAlpacaCredentials(service, user)
		if err == nil {
			if key == "" {
				key = storedKey
			}
			if sec == "" {
				sec = storedSecret
			}
			source = "keyring"
		} else if !errors.Is(err, keyring.ErrNotFound) {
			return "", "", "", fmt.Errorf("load keyring credentials: %w", err)
		}
	}

	if key == "" || sec == "" {
		return "", "", "", fmt.Errorf("alpaca API key and secret are required (flags/env missing and no keyring entry found)")
	}

	if cfg.Save && (strings.TrimSpace(apiKey) != "" || strings.TrimSpace(secret) != "") {
		if err := SaveAlpacaCredentials(service, user, key, sec); err != nil {
			return "", "", "", fmt.Errorf("save keyring credentials: %w", err)
		}
	}

	return key, sec, source, nil
}

func LoadAlpacaCredentials(service, user string) (string, string, error) {
	key, err := keyring.Get(serviceName(service), accountName(userName(user), keyIDField))
	if err != nil {
		return "", "", err
	}
	secret, err := keyring.Get(serviceName(service), accountName(userName(user), secretField))
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(key), strings.TrimSpace(secret), nil
}

func SaveAlpacaCredentials(service, user, apiKey, apiSecret string) error {
	if err := keyring.Set(serviceName(service), accountName(userName(user), keyIDField), strings.TrimSpace(apiKey)); err != nil {
		return err
	}
	if err := keyring.Set(serviceName(service), accountName(userName(user), secretField), strings.TrimSpace(apiSecret)); err != nil {
		return err
	}
	return nil
}

func serviceName(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return DefaultService
	}
	return in
}

func userName(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return DefaultUser
	}
	return in
}

func accountName(user, field string) string {
	return user + ":" + field
}
