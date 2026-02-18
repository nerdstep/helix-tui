package credentials

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestResolveAlpacaCredentials_DisabledRequiresBoth(t *testing.T) {
	_, _, _, err := ResolveAlpacaCredentials("", "", KeyringConfig{Enabled: false})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveAlpacaCredentials_LoadsFromKeyring(t *testing.T) {
	restore := stubKeyringFuncs(
		func(service, user string) (string, error) {
			switch user {
			case "paper:alpaca_api_key_id":
				return " key123 ", nil
			case "paper:alpaca_api_secret_key":
				return " sec123 ", nil
			default:
				return "", keyring.ErrNotFound
			}
		},
		func(string, string, string) error { return nil },
	)
	defer restore()

	key, sec, source, err := ResolveAlpacaCredentials("", "", KeyringConfig{
		Enabled: true,
		Service: "svc",
		User:    "paper",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "key123" || sec != "sec123" {
		t.Fatalf("unexpected key/secret: %q / %q", key, sec)
	}
	if source != "keyring" {
		t.Fatalf("unexpected source: %q", source)
	}
}

func TestResolveAlpacaCredentials_UsesProvidedAndSaves(t *testing.T) {
	var calls []string
	restore := stubKeyringFuncs(
		func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		},
		func(service, user, value string) error {
			calls = append(calls, fmt.Sprintf("%s|%s|%s", service, user, value))
			return nil
		},
	)
	defer restore()

	key, sec, source, err := ResolveAlpacaCredentials(" key-flag ", " sec-flag ", KeyringConfig{
		Enabled: true,
		Save:    true,
		Service: "svc",
		User:    "paper",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "key-flag" || sec != "sec-flag" {
		t.Fatalf("unexpected key/secret: %q / %q", key, sec)
	}
	if source != "flags/env" {
		t.Fatalf("unexpected source: %q", source)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 keyring set calls, got %d", len(calls))
	}
}

func TestResolveAlpacaCredentials_NotFoundStillRequiresValues(t *testing.T) {
	restore := stubKeyringFuncs(
		func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		},
		func(string, string, string) error { return nil },
	)
	defer restore()

	_, _, _, err := ResolveAlpacaCredentials("", "", KeyringConfig{Enabled: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "no keyring entry found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveAlpacaCredentials_LoadFailure(t *testing.T) {
	restore := stubKeyringFuncs(
		func(string, string) (string, error) {
			return "", errors.New("boom")
		},
		func(string, string, string) error { return nil },
	)
	defer restore()

	_, _, _, err := ResolveAlpacaCredentials("", "", KeyringConfig{Enabled: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "load keyring credentials") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubKeyringFuncs(
	get func(service, user string) (string, error),
	set func(service, user, value string) error,
) func() {
	oldGet := keyringGet
	oldSet := keyringSet
	keyringGet = get
	keyringSet = set
	return func() {
		keyringGet = oldGet
		keyringSet = oldSet
	}
}
