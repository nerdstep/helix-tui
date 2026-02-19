package tui

import (
	"context"
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
	if err := m.engine.CancelOrder(ctx, cmd.OrderID); err != nil {
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
