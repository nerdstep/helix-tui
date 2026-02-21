package strategy

import "testing"

func TestNormalizedStrategyIdentityDefaults(t *testing.T) {
	identity := normalizedStrategyIdentity("", "", "")
	if identity.HumanName != "Operator" {
		t.Fatalf("unexpected default human name: %q", identity.HumanName)
	}
	if identity.AgentName != "Helix" {
		t.Fatalf("unexpected default agent name: %q", identity.AgentName)
	}
	if identity.HumanAlias != "" {
		t.Fatalf("unexpected default human alias: %q", identity.HumanAlias)
	}
}

func TestBuildStrategyIdentitySystemPrompt(t *testing.T) {
	identity := strategyIdentityInput{
		HumanName:  "Justin Doe",
		HumanAlias: "@justwebdev",
		AgentName:  "Athena",
	}
	got := buildStrategyIdentitySystemPrompt("Focus on risk-aware, concise plans.", identity)
	wantPrefix := "Identity context: You are Athena, the strategy analyst for Justin Doe (@justwebdev)."
	if len(got) <= len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("expected prompt to include identity prefix, got: %q", got)
	}
}
