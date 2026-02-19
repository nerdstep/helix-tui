package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	vm := m.buildViewModel()
	left := panelStyle.Render(strings.Join(vm.positions, "\n"))
	mid := panelStyle.Render(strings.Join(vm.orders, "\n"))
	right := panelStyle.Render(strings.Join(vm.watchlist, "\n"))
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	midRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		panelStyle.Render(strings.Join(vm.pnl, "\n")),
		panelStyle.Render(strings.Join(vm.system, "\n")),
	)

	statusRenderer := okStyle
	if vm.statusError {
		statusRenderer = errStyle
	}
	status := statusRenderer.Render(vm.status)
	input := inputStyle.Render("> " + vm.input)

	return strings.Join([]string{
		vm.header,
		vm.account,
		top,
		midRow,
		panelStyle.Render(strings.Join(vm.events, "\n")),
		status,
		input,
		vm.footer,
	}, "\n")
}
