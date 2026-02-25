package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/util"
)

func (m Model) eventPageSize() int {
	if m.activeTab == tabLogs && m.height > 0 {
		return util.MaxInt(1, m.height-m.logsReservedHeight())
	}
	return 8
}

func (m Model) logsReservedHeight() int {
	headerHeight := lipgloss.Height(m.buildHeader())
	if headerHeight < 1 {
		headerHeight = 1
	}
	tabHeight := lipgloss.Height(m.renderTabBar(m.computeLayoutSpec().usableWidth))
	if tabHeight < 1 {
		tabHeight = 1
	}
	statusInputHeight := m.statusInputHeight()
	footerHeight := util.MaxInt(1, m.footerHeight())
	// panel title + viewport hint plus panel borders.
	panelOverhead := panelStyle.GetVerticalFrameSize() + 2
	// Keep one extra row to absorb terminal wrapping edge-cases.
	safetyPadding := 1
	return headerHeight + tabHeight + statusInputHeight + footerHeight + panelOverhead + safetyPadding
}

func (m Model) maxEventScroll() int {
	total := m.eventLineCount()
	visible := m.eventPageSize()
	if total <= visible {
		return 0
	}
	return total - visible
}

func (m Model) statusInputHeight() int {
	statusRenderer := okStyle
	if m.statusError {
		statusRenderer = errStyle
	} else if m.isLoading() {
		statusRenderer = warnStyle
	}
	statusText := m.status
	if m.isLoading() {
		statusText = m.spinner.View() + " " + m.status
	}
	statusLine := statusRenderer.Render(statusText)
	inputLine := inputStyle.Render("> " + m.input)
	h := lipgloss.Height(statusLine) + lipgloss.Height(inputLine)
	return util.MaxInt(2, h)
}

func (m *Model) clampEventScroll() {
	if m.eventScroll < 0 {
		m.eventScroll = 0
		return
	}
	max := m.maxEventScroll()
	if m.eventScroll > max {
		m.eventScroll = max
	}
}

func (m *Model) setEventScroll(next int) {
	m.eventScroll = next
	m.clampEventScroll()
	m.applyEventScrollToViewport()
}

func (m *Model) scrollEvents(delta int) {
	m.setEventScroll(m.eventScroll + delta)
	m.status = m.eventScrollStatus()
	m.statusError = false
}

func (m Model) eventWindow() (start, end, total int) {
	total = m.eventLineCount()
	if total == 0 {
		return 0, 0, 0
	}
	visible := m.eventPageSize()
	end = total - m.eventScroll
	if end < 0 {
		end = 0
	}
	start = end - visible
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	return start, end, total
}

func (m Model) eventLineCount() int {
	if m.eventLinesReady && m.eventLinesEvents == len(m.snapshot.Events) {
		return len(m.eventLines)
	}
	return len(m.snapshot.Events)
}

func (m *Model) applyEventScrollToViewport() {
	max := m.maxEventScroll()
	offset := max - m.eventScroll
	m.eventsViewport.SetYOffset(offset)
}

func (m Model) eventScrollStatus() string {
	start, end, total := m.eventWindow()
	if total == 0 {
		return "events: (none)"
	}
	return fmt.Sprintf("events: showing %d-%d of %d", start+1, end, total)
}
