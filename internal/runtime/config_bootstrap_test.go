package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeBootstrapConfigWithPromptCreatesConfigOnAccept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	out := &bytes.Buffer{}
	err := maybeBootstrapConfigWithPrompt(path, strings.NewReader("\n"), out, true)
	if err != nil {
		t.Fatalf("maybeBootstrapConfigWithPrompt failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected created config file: %v", err)
	}
	if strings.TrimSpace(string(content)) == "" {
		t.Fatalf("expected non-empty config file")
	}
	if !strings.Contains(out.String(), "created") {
		t.Fatalf("expected created output, got: %q", out.String())
	}
}

func TestMaybeBootstrapConfigWithPromptSkipsOnDecline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	out := &bytes.Buffer{}
	err := maybeBootstrapConfigWithPrompt(path, strings.NewReader("n\n"), out, true)
	if err != nil {
		t.Fatalf("maybeBootstrapConfigWithPrompt failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config file to remain absent, got err=%v", err)
	}
	if !strings.Contains(out.String(), "continuing without config file") {
		t.Fatalf("expected skip output, got: %q", out.String())
	}
}

func TestMaybeBootstrapConfigWithPromptNonInteractiveNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	out := &bytes.Buffer{}
	err := maybeBootstrapConfigWithPrompt(path, strings.NewReader(""), out, false)
	if err != nil {
		t.Fatalf("maybeBootstrapConfigWithPrompt failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected config file to remain absent, got err=%v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt output for non-interactive mode, got: %q", out.String())
	}
}

func TestWriteBootstrapConfigFallsBackWhenTemplateMissing(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	path := filepath.Join(temp, "config.toml")
	source, err := writeBootstrapConfig(path)
	if err != nil {
		t.Fatalf("writeBootstrapConfig failed: %v", err)
	}
	if source != "built-in fallback template" {
		t.Fatalf("unexpected source: %q", source)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), `mode = "manual"`) {
		t.Fatalf("expected fallback template content, got: %q", string(content))
	}
}

func TestMaybeBootstrapConfigExplicitPathSkipsPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")
	out := &bytes.Buffer{}
	err := maybeBootstrapConfig(path, true, strings.NewReader("\n"), out)
	if err != nil {
		t.Fatalf("maybeBootstrapConfig failed: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output for explicit path, got: %q", out.String())
	}
}
