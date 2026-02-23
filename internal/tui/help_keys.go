package tui

import "github.com/charmbracelet/bubbles/key"

type dashboardKeyMap struct {
	Buy          key.Binding
	Sell         key.Binding
	Cancel       key.Binding
	Flatten      key.Binding
	Sync         key.Binding
	Watch        key.Binding
	Strategy     key.Binding
	Events       key.Binding
	TabCmd       key.Binding
	TabKey       key.Binding
	ScrollUp     key.Binding
	ScrollDn     key.Binding
	PageUp       key.Binding
	PageDn       key.Binding
	Home         key.Binding
	End          key.Binding
	ToggleHelp   key.Binding
	QuitCmd      key.Binding
	QuitKeyboard key.Binding
}

func newDashboardKeyMap() dashboardKeyMap {
	return dashboardKeyMap{
		Buy: key.NewBinding(
			key.WithKeys("buy <sym> <qty>"),
			key.WithHelp("buy <sym> <qty>", "market buy"),
		),
		Sell: key.NewBinding(
			key.WithKeys("sell <sym> <qty>"),
			key.WithHelp("sell <sym> <qty>", "market sell"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("cancel <id|prefix|#row>"),
			key.WithHelp("cancel <id|prefix|#row>", "cancel order"),
		),
		Flatten: key.NewBinding(
			key.WithKeys("flatten"),
			key.WithHelp("flatten", "close positions"),
		),
		Sync: key.NewBinding(
			key.WithKeys("sync"),
			key.WithHelp("sync", "refresh account"),
		),
		Watch: key.NewBinding(
			key.WithKeys("watch list|add|remove|sync"),
			key.WithHelp("watch list|add|remove|sync", "watchlist"),
		),
		Strategy: key.NewBinding(
			key.WithKeys("strategy run|status|approve|reject|archive"),
			key.WithHelp("strategy ...", "strategy plans"),
		),
		Events: key.NewBinding(
			key.WithKeys("events up|down|top|tail [n]"),
			key.WithHelp("events ...", "scroll events"),
		),
		TabCmd: key.NewBinding(
			key.WithKeys("tab overview|strategy|system|logs"),
			key.WithHelp("tab <name>", "switch tab"),
		),
		TabKey: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("up", "scroll up"),
		),
		ScrollDn: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("down", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDn: key.NewBinding(
			key.WithKeys("pgdn"),
			key.WithHelp("pgdn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		End: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "tail"),
		),
		ToggleHelp: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		QuitCmd: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
		QuitKeyboard: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "force quit"),
		),
	}
}

func (k dashboardKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.ToggleHelp,
		k.QuitCmd,
		k.TabKey,
	}
}

func (k dashboardKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Buy, k.Sell, k.Cancel, k.Flatten, k.Sync},
		{k.Watch, k.Strategy, k.Events, k.TabCmd, k.QuitCmd},
		{k.TabKey, k.ScrollUp, k.ScrollDn, k.PageUp, k.PageDn, k.Home, k.End, k.ToggleHelp, k.QuitKeyboard},
	}
}
