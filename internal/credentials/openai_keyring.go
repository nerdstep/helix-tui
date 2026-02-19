package credentials

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const openAIKeyField = "openai_api_key"

func ResolveOpenAICredentials(apiKey string, cfg KeyringConfig) (string, string, error) {
	key := strings.TrimSpace(apiKey)
	source := "flags/env"

	if !cfg.Enabled {
		if key == "" {
			return "", "", fmt.Errorf("llm api key is required")
		}
		return key, source, nil
	}

	service := serviceName(cfg.Service)
	user := userName(cfg.User)

	if key == "" {
		storedKey, err := LoadOpenAICredentials(service, user)
		if err == nil {
			key = storedKey
			source = "keyring"
		} else if !errors.Is(err, keyring.ErrNotFound) {
			return "", "", fmt.Errorf("load keyring credentials: %w", err)
		}
	}

	if key == "" {
		return "", "", fmt.Errorf("llm api key is required (flags/env missing and no keyring entry found)")
	}

	if cfg.Save && strings.TrimSpace(apiKey) != "" {
		if err := SaveOpenAICredentials(service, user, key); err != nil {
			return "", "", fmt.Errorf("save keyring credentials: %w", err)
		}
	}
	return key, source, nil
}

func LoadOpenAICredentials(service, user string) (string, error) {
	key, err := keyringGet(serviceName(service), accountName(userName(user), openAIKeyField))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(key), nil
}

func SaveOpenAICredentials(service, user, apiKey string) error {
	return keyringSet(
		serviceName(service),
		accountName(userName(user), openAIKeyField),
		strings.TrimSpace(apiKey),
	)
}
