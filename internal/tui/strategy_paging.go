package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"helix-tui/internal/util"
)

func (m Model) strategyPageSize() int {
	if m.activeTab != tabStrategy || m.height <= 0 {
		return 8
	}

	spec := m.computeLayoutSpec()
	headerHeight := util.MaxInt(1, lipgloss.Height(m.buildHeader()))
	tabHeight := util.MaxInt(1, lipgloss.Height(m.renderTabBar(spec.usableWidth)))
	footerHeight := util.MaxInt(1, m.footerHeight())
	statusInputHeight := 2
	gapCount := 2
	otherPanelHeight := m.strategyTwoColumnOtherPanelsHeight(spec)
	if !spec.twoColumn {
		otherPanelHeight = m.strategySingleColumnOtherPanelsHeight(spec)
		gapCount = 3
	}

	availablePanelHeight := m.height - headerHeight - tabHeight - statusInputHeight - footerHeight - gapCount - otherPanelHeight
	inner := availablePanelHeight - panelStyle.GetVerticalFrameSize()
	return util.MaxInt(3, inner)
}

func (m Model) strategyChatPageSize() int {
	if m.activeTab != tabChat || m.height <= 0 {
		return 8
	}
	return util.MaxInt(3, m.height-m.strategyChatReservedHeight())
}

func (m Model) strategyChatReservedHeight() int {
	headerHeight := util.MaxInt(1, lipgloss.Height(m.buildHeader()))
	tabHeight := util.MaxInt(1, lipgloss.Height(m.renderTabBar(m.computeLayoutSpec().usableWidth)))
	statusInputHeight := 2
	footerHeight := util.MaxInt(1, m.footerHeight())
	// Panel title + viewport hint + panel borders.
	panelOverhead := panelStyle.GetVerticalFrameSize() + 2
	safetyPadding := 1
	return headerHeight + tabHeight + statusInputHeight + footerHeight + panelOverhead + safetyPadding
}

func (m Model) strategyTwoColumnOtherPanelsHeight(spec layoutSpec) int {
	overview := lipglossHeightForPanel(m.buildStrategyOverviewRows(), spec.usableWidth)
	health := lipglossHeightForPanel(m.buildStrategyHealthRows(), spec.rightWidth)
	recent := lipglossHeightForPanel(m.buildStrategyRecentRows(), spec.leftWidth)
	return overview + util.MaxInt(health, recent)
}

func (m Model) strategySingleColumnOtherPanelsHeight(spec layoutSpec) int {
	overview := lipglossHeightForPanel(m.buildStrategyOverviewRows(), spec.usableWidth)
	health := lipglossHeightForPanel(m.buildStrategyHealthRows(), spec.usableWidth)
	recent := lipglossHeightForPanel(m.buildStrategyRecentRows(), spec.usableWidth)
	return overview + health + recent
}

func (m Model) footerHeight() int {
	footer := m.buildFooter()
	if footer == "" {
		return 1
	}
	return lipgloss.Height(footer)
}

func (m *Model) scrollStrategyLine(delta int) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		m.strategyViewport.LineUp(delta)
	} else {
		m.strategyViewport.LineDown(-delta)
	}
	m.status = m.strategyScrollStatus()
	m.statusError = false
}

func (m *Model) scrollStrategyPage(delta int) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		m.strategyViewport.ViewUp()
	} else {
		m.strategyViewport.ViewDown()
	}
	m.status = m.strategyScrollStatus()
	m.statusError = false
}

func (m *Model) setStrategyScrollTop() {
	m.strategyViewport.GotoTop()
	m.status = m.strategyScrollStatus()
	m.statusError = false
}

func (m *Model) setStrategyScrollBottom() {
	m.strategyViewport.GotoBottom()
	m.status = m.strategyScrollStatus()
	m.statusError = false
}

func (m *Model) scrollStrategyChatLine(delta int) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		m.strategyChatViewport.LineUp(delta)
	} else {
		m.strategyChatViewport.LineDown(-delta)
	}
	m.status = m.strategyChatScrollStatus()
	m.statusError = false
}

func (m *Model) scrollStrategyChatPage(delta int) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		m.strategyChatViewport.ViewUp()
	} else {
		m.strategyChatViewport.ViewDown()
	}
	m.status = m.strategyChatScrollStatus()
	m.statusError = false
}

func (m *Model) setStrategyChatScrollTop() {
	m.strategyChatViewport.GotoTop()
	m.status = m.strategyChatScrollStatus()
	m.statusError = false
}

func (m *Model) setStrategyChatScrollBottom() {
	m.strategyChatViewport.GotoBottom()
	m.status = m.strategyChatScrollStatus()
	m.statusError = false
}

func (m Model) strategyWindow() (start, end, total int) {
	total = m.strategyViewport.TotalLineCount()
	if total == 0 {
		return 0, 0, 0
	}
	start = m.strategyViewport.YOffset
	if start < 0 {
		start = 0
	}
	visible := util.MaxInt(1, m.strategyViewport.VisibleLineCount())
	end = start + visible
	if end > total {
		end = total
	}
	return start, end, total
}

func (m Model) strategyScrollStatus() string {
	start, end, total := m.strategyWindow()
	if total == 0 {
		return "strategy: no recommendations"
	}
	return fmt.Sprintf("strategy: showing %d-%d of %d", start+1, end, total)
}

func (m Model) strategyChatWindow() (start, end, total int) {
	total = m.strategyChatViewport.TotalLineCount()
	if total == 0 {
		return 0, 0, 0
	}
	start = m.strategyChatViewport.YOffset
	if start < 0 {
		start = 0
	}
	visible := util.MaxInt(1, m.strategyChatViewport.VisibleLineCount())
	end = start + visible
	if end > total {
		end = total
	}
	return start, end, total
}

func (m Model) strategyChatScrollStatus() string {
	start, end, total := m.strategyChatWindow()
	if total == 0 {
		return "chat: no messages"
	}
	return fmt.Sprintf("chat: showing %d-%d of %d", start+1, end, total)
}

func lipglossHeightForPanel(lines []string, width int) int {
	return lipgloss.Height(renderPanel(lines, width))
}
