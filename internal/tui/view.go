package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	vm := m.buildViewModel()
	gap := 1
	spec := m.computeLayoutSpec()
	tabBar := m.renderTabBar(spec.usableWidth)
	content := m.renderTabContent(vm, spec, gap)

	statusRenderer := okStyle
	if vm.statusError {
		statusRenderer = errStyle
	}
	status := statusRenderer.Render(vm.status)
	input := inputStyle.Render("> " + vm.input)

	lines := []string{vm.header, tabBar}
	if vm.account != "" {
		lines = append(lines, vm.account)
	}
	lines = append(lines,
		content,
		status,
		input,
		vm.footer,
	)
	return strings.Join(lines, "\n")
}

func (m Model) renderTabContent(vm viewModel, spec layoutSpec, gap int) string {
	if m.activeTab == tabLogs {
		return renderPanel(vm.events, spec.usableWidth)
	}
	if m.activeTab == tabSystem {
		return renderPanel(vm.system, spec.usableWidth)
	}
	if spec.twoColumn {
		row1 := renderPanel(vm.watchlist, spec.usableWidth)
		row2 := renderTwoColumnPanels(vm.positions, vm.orders, spec.leftWidth, spec.rightWidth, spec.usableWidth, gap)
		row3 := renderTwoColumnPanels(vm.pnl, vm.momentum, spec.leftWidth, spec.rightWidth, spec.usableWidth, gap)
		return lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		renderPanel(vm.watchlist, spec.usableWidth),
		renderPanel(vm.positions, spec.usableWidth),
		renderPanel(vm.orders, spec.usableWidth),
		renderPanel(vm.pnl, spec.usableWidth),
		renderPanel(vm.momentum, spec.usableWidth),
	)
}

func renderTwoColumnPanels(leftLines []string, rightLines []string, leftWidth int, rightWidth int, total int, gap int) string {
	left := renderPanel(leftLines, leftWidth)
	right := renderPanel(rightLines, rightWidth)
	row := joinHorizontalWithGap([]string{left, right}, gap)
	delta := total - lipgloss.Width(row)
	if delta != 0 {
		right = renderPanel(rightLines, maxInt(1, rightWidth+delta))
		row = joinHorizontalWithGap([]string{left, right}, gap)
	}
	return row
}

func renderPanel(lines []string, width int) string {
	style := panelStyle
	if width > 0 {
		frame := style.GetHorizontalFrameSize()
		inner := maxInt(1, width-frame)
		lineStyle := lipgloss.NewStyle().Width(inner).MaxWidth(inner)
		clamped := make([]string, 0, len(lines))
		for _, line := range lines {
			clamped = append(clamped, lineStyle.Render(line))
		}
		style = style.Width(inner)
		return style.Render(strings.Join(clamped, "\n"))
	}
	return style.Render(strings.Join(lines, "\n"))
}

func splitEvenWidths(total, cols, gap int) []int {
	if cols <= 0 {
		return nil
	}
	available := total - gap*(cols-1)
	if available < cols {
		available = cols
	}
	base := available / cols
	rem := available % cols
	out := make([]int, cols)
	for i := 0; i < cols; i++ {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out
}

func joinHorizontalWithGap(blocks []string, gap int) string {
	if len(blocks) == 0 {
		return ""
	}
	if gap <= 0 {
		return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
	}
	parts := make([]string, 0, len(blocks)*2-1)
	sep := strings.Repeat(" ", gap)
	for i, b := range blocks {
		if i > 0 {
			parts = append(parts, sep)
		}
		parts = append(parts, b)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}
