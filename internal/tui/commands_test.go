package tui

import (
	"strings"
	"testing"

	"helix-tui/internal/domain"
)

func TestResolveCancelOrderID_RowSelector(t *testing.T) {
	orders := []domain.Order{
		{ID: "11111111-1111-1111-1111-111111111111"},
		{ID: "22222222-2222-2222-2222-222222222222"},
	}
	got, err := resolveCancelOrderID("#2", orders)
	if err != nil {
		t.Fatalf("resolveCancelOrderID returned error: %v", err)
	}
	if got != orders[1].ID {
		t.Fatalf("unexpected resolved id: got %q want %q", got, orders[1].ID)
	}
}

func TestResolveCancelOrderID_PrefixAndExact(t *testing.T) {
	orders := []domain.Order{
		{ID: "aaaaaaaa-1111-1111-1111-111111111111"},
		{ID: "aaaabbbb-2222-2222-2222-222222222222"},
		{ID: "bbbbbbbb-3333-3333-3333-333333333333"},
	}

	got, err := resolveCancelOrderID("bbbb", orders)
	if err != nil {
		t.Fatalf("expected unique prefix to resolve, got error: %v", err)
	}
	if got != orders[2].ID {
		t.Fatalf("unexpected resolved id: got %q want %q", got, orders[2].ID)
	}

	got, err = resolveCancelOrderID("aaaabbbb-2222-2222-2222-222222222222", orders)
	if err != nil {
		t.Fatalf("expected exact id to resolve, got error: %v", err)
	}
	if got != orders[1].ID {
		t.Fatalf("unexpected exact resolved id: got %q want %q", got, orders[1].ID)
	}
}

func TestResolveCancelOrderID_AmbiguousOrMissing(t *testing.T) {
	orders := []domain.Order{
		{ID: "aaaaaaaa-1111-1111-1111-111111111111"},
		{ID: "aaaabbbb-2222-2222-2222-222222222222"},
	}

	_, err := resolveCancelOrderID("aaaa", orders)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous prefix error, got %v", err)
	}

	_, err = resolveCancelOrderID("zzzz", orders)
	if err == nil || !strings.Contains(err.Error(), "no open order matches") {
		t.Fatalf("expected no-match error, got %v", err)
	}

	_, err = resolveCancelOrderID("#9", orders)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
}
