package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	vm := m.buildViewModel()
	gap := 1
	contentWidth := m.width
	if contentWidth <= 0 {
		contentWidth = 132
	}
	usableWidth := maxInt(24, contentWidth-1)
	minPanelWidth := 32
	var body string
	if usableWidth >= minPanelWidth*2+gap {
		colWidths := splitEvenWidths(usableWidth, 2, gap)
		row1 := joinHorizontalWithGap([]string{
			renderPanel(vm.positions, colWidths[0]),
			renderPanel(vm.orders, colWidths[1]),
		}, gap)
		row2 := renderPanel(vm.watchlist, usableWidth)
		row3 := joinHorizontalWithGap([]string{
			renderPanel(vm.pnl, colWidths[0]),
			renderPanel(vm.system, colWidths[1]),
		}, gap)
		body = lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3)
	} else {
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			renderPanel(vm.positions, usableWidth),
			renderPanel(vm.orders, usableWidth),
			renderPanel(vm.watchlist, usableWidth),
			renderPanel(vm.pnl, usableWidth),
			renderPanel(vm.system, usableWidth),
		)
	}

	statusRenderer := okStyle
	if vm.statusError {
		statusRenderer = errStyle
	}
	status := statusRenderer.Render(vm.status)
	input := inputStyle.Render("> " + vm.input)

	return strings.Join([]string{
		vm.header,
		vm.account,
		body,
		renderPanel(vm.events, usableWidth),
		status,
		input,
		vm.footer,
	}, "\n")
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
