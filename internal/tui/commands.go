package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"helix-tui/internal/domain"
)

func (m Model) runCommand(raw string) tea.Cmd {
	return func() tea.Msg {
		parsed, parseErr := parseCoreCommand(raw)
		if parseErr != nil {
			return *parseErr
		}
		if parsed == nil {
			return refreshMsg{snapshot: m.engine.Snapshot()}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.executeCoreCommand(ctx, *parsed)
	}
}

func (m Model) executeCoreCommand(ctx context.Context, cmd coreCommand) tea.Msg {
	switch cmd.Type {
	case coreCommandQuit:
		return quitMsg{}
	case coreCommandHelp:
		return statusOnlyMsg{status: helpCommandText}
	case coreCommandSync:
		return m.executeSync(ctx)
	case coreCommandFlatten:
		return m.executeFlatten(ctx)
	case coreCommandCancel:
		return m.executeCancel(ctx, cmd)
	case coreCommandOrder:
		return m.executeOrder(ctx, cmd)
	default:
		return statusOnlyMsg{status: "unknown command; type help", isErr: true}
	}
}

func (m Model) executeSync(ctx context.Context) tea.Msg {
	if err := m.engine.Sync(ctx); err != nil {
		return statusOnlyMsg{status: err.Error(), isErr: true}
	}
	return statusOnlyMsg{status: "sync complete", refresh: true}
}

func (m Model) executeFlatten(ctx context.Context) tea.Msg {
	if err := m.engine.Flatten(ctx); err != nil {
		return statusOnlyMsg{status: err.Error(), isErr: true}
	}
	return statusOnlyMsg{status: "flatten orders submitted", refresh: true}
}

func (m Model) executeCancel(ctx context.Context, cmd coreCommand) tea.Msg {
	orderID, err := resolveCancelOrderID(cmd.OrderID, m.snapshot.Orders)
	if err != nil {
		return statusOnlyMsg{status: err.Error(), isErr: true}
	}
	if err := m.engine.CancelOrder(ctx, orderID); err != nil {
		return statusOnlyMsg{status: err.Error(), isErr: true}
	}
	return statusOnlyMsg{status: "cancel requested", refresh: true}
}

func (m Model) executeOrder(ctx context.Context, cmd coreCommand) tea.Msg {
	_, err := m.engine.PlaceOrder(ctx, domain.OrderRequest{
		Symbol: cmd.Symbol,
		Side:   cmd.Side,
		Qty:    cmd.Qty,
		Type:   domain.OrderTypeMarket,
	})
	if err != nil {
		return statusOnlyMsg{status: err.Error(), isErr: true}
	}
	return statusOnlyMsg{status: "order submitted", refresh: true}
}

func resolveCancelOrderID(token string, orders []domain.Order) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("usage: cancel <ORDER_ID|ORDER_ID_PREFIX|#ROW>")
	}
	if len(orders) == 0 {
		return "", fmt.Errorf("no open orders to cancel")
	}
	if strings.HasPrefix(token, "#") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "#")))
		if err != nil || n <= 0 {
			return "", fmt.Errorf("invalid order row %q; use #<row>", token)
		}
		if n > len(orders) {
			return "", fmt.Errorf("order row %d out of range (1-%d)", n, len(orders))
		}
		return orders[n-1].ID, nil
	}

	for _, o := range orders {
		if strings.EqualFold(o.ID, token) {
			return o.ID, nil
		}
	}

	prefixMatches := make([]string, 0, 3)
	lowerToken := strings.ToLower(token)
	matchCount := 0
	for _, o := range orders {
		if strings.HasPrefix(strings.ToLower(o.ID), lowerToken) {
			matchCount++
			if len(prefixMatches) < cap(prefixMatches) {
				prefixMatches = append(prefixMatches, o.ID)
			}
		}
	}
	if matchCount == 1 {
		return prefixMatches[0], nil
	}
	if matchCount > 1 {
		extra := ""
		if matchCount > len(prefixMatches) {
			extra = ", ..."
		}
		return "", fmt.Errorf(
			"ambiguous order prefix %q (%d matches: %s%s); use more characters or cancel #<row>",
			token,
			matchCount,
			strings.Join(prefixMatches, ", "),
			extra,
		)
	}
	return "", fmt.Errorf("no open order matches %q; use cancel #<row> or run sync", token)
}
