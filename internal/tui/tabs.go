package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const tabCommandUsage = "usage: tab <overview|logs|strategy|system>"

func (m *Model) handleTabCommand(raw string) (bool, tea.Cmd) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(fields) == 0 || fields[0] != "tab" {
		return false, nil
	}
	if len(fields) == 1 {
		m.toggleTab()
		return true, nil
	}
	if len(fields) != 2 {
		m.setStatus(tabCommandUsage, true)
		return true, nil
	}
	switch fields[1] {
	case string(tabOverview), "dashboard":
		m.setActiveTab(tabOverview)
	case string(tabLogs), "events":
		m.setActiveTab(tabLogs)
	case string(tabStrategy):
		m.setActiveTab(tabStrategy)
	case string(tabSystem):
		m.setActiveTab(tabSystem)
	default:
		m.setStatus(tabCommandUsage, true)
	}
	return true, nil
}

func (m *Model) toggleTab() {
	if m.activeTab == tabOverview {
		m.setActiveTab(tabLogs)
		return
	}
	if m.activeTab == tabLogs {
		m.setActiveTab(tabStrategy)
		return
	}
	if m.activeTab == tabStrategy {
		m.setActiveTab(tabSystem)
		return
	}
	if m.activeTab == tabSystem {
		m.setActiveTab(tabOverview)
		return
	}
	m.setActiveTab(tabOverview)
}

func (m *Model) setActiveTab(tab uiTab) {
	m.activeTab = tab
	m.syncEventsViewport()
	m.setStatus("tab: "+string(tab), false)
}

func (m Model) renderTabBar(width int) string {
	overview := tabInactiveStyle.Render("Overview")
	logs := tabInactiveStyle.Render("Logs")
	strategy := tabInactiveStyle.Render("Strategy")
	system := tabInactiveStyle.Render("System")
	if m.activeTab == tabOverview {
		overview = tabActiveStyle.Render("Overview")
	}
	if m.activeTab == tabLogs {
		logs = tabActiveStyle.Render("Logs")
	}
	if m.activeTab == tabStrategy {
		strategy = tabActiveStyle.Render("Strategy")
	}
	if m.activeTab == tabSystem {
		system = tabActiveStyle.Render("System")
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Left, overview, " ", logs, " ", strategy, " ", system)
	if width <= 0 {
		return bar
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(bar)
}
