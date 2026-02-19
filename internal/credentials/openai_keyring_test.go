package credentials

import (
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestResolveOpenAICredentials_DisabledRequiresKey(t *testing.T) {
	_, _, err := ResolveOpenAICredentials("", KeyringConfig{Enabled: false})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveOpenAICredentials_LoadsFromKeyring(t *testing.T) {
	restore := stubKeyringFuncs(
		func(service, user string) (string, error) {
			if user == "paper:openai_api_key" {
				return " sk-test ", nil
			}
			return "", keyring.ErrNotFound
		},
		func(string, string, string) error { return nil },
	)
	defer restore()

	key, source, err := ResolveOpenAICredentials("", KeyringConfig{
		Enabled: true,
		Service: "svc",
		User:    "paper",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-test" {
		t.Fatalf("unexpected key: %q", key)
	}
	if source != "keyring" {
		t.Fatalf("unexpected source: %q", source)
	}
}

func TestResolveOpenAICredentials_UsesProvidedAndSaves(t *testing.T) {
	calls := 0
	restore := stubKeyringFuncs(
		func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		},
		func(string, string, string) error {
			calls++
			return nil
		},
	)
	defer restore()

	key, source, err := ResolveOpenAICredentials(" sk-live ", KeyringConfig{
		Enabled: true,
		Save:    true,
		Service: "svc",
		User:    "paper",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-live" {
		t.Fatalf("unexpected key: %q", key)
	}
	if source != "flags/env" {
		t.Fatalf("unexpected source: %q", source)
	}
	if calls != 1 {
		t.Fatalf("expected one save call, got %d", calls)
	}
}

func TestResolveOpenAICredentials_NotFoundStillRequiresKey(t *testing.T) {
	restore := stubKeyringFuncs(
		func(string, string) (string, error) {
			return "", keyring.ErrNotFound
		},
		func(string, string, string) error { return nil },
	)
	defer restore()

	_, _, err := ResolveOpenAICredentials("", KeyringConfig{Enabled: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "no keyring entry found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveOpenAICredentials_LoadFailure(t *testing.T) {
	restore := stubKeyringFuncs(
		func(string, string) (string, error) {
			return "", errors.New("boom")
		},
		func(string, string, string) error { return nil },
	)
	defer restore()

	_, _, err := ResolveOpenAICredentials("", KeyringConfig{Enabled: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "load keyring credentials") {
		t.Fatalf("unexpected error: %v", err)
	}
}
