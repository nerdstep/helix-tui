package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) eventPageSize() int {
	if m.activeTab == tabLogs && m.height > 0 {
		return maxInt(1, m.height-m.logsReservedHeight())
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
	// status + input + footer plus panel borders and static rows around viewport.
	return headerHeight + tabHeight + 5 + panelStyle.GetVerticalFrameSize() + 2
}

func (m Model) maxEventScroll() int {
	total := len(m.snapshot.Events)
	visible := m.eventPageSize()
	if total <= visible {
		return 0
	}
	return total - visible
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
	total = len(m.snapshot.Events)
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
