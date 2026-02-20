package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"helix-tui/internal/domain"
)

const (
	watchlistStateStaleAfter = 20 * time.Second
)

type layoutSpec struct {
	usableWidth int
	twoColumn   bool
	leftWidth   int
	rightWidth  int
}

func (m Model) computeLayoutSpec() layoutSpec {
	const (
		gap           = 1
		minPanelWidth = 32
		defaultWidth  = 132
	)
	contentWidth := m.width
	if contentWidth <= 0 {
		contentWidth = defaultWidth
	}
	usableWidth := maxInt(24, contentWidth-1)
	spec := layoutSpec{usableWidth: usableWidth}
	if usableWidth >= minPanelWidth*2+gap {
		spec.twoColumn = true
		colWidths := splitEvenWidths(usableWidth, 2, gap)
		colWidths = adjustPanelWidthsToTotal(colWidths, gap, usableWidth)
		spec.leftWidth = colWidths[0]
		spec.rightWidth = colWidths[1]
		return spec
	}
	spec.leftWidth = usableWidth
	spec.rightWidth = usableWidth
	return spec
}

func panelInnerWidth(panelWidth int) int {
	return maxInt(1, panelWidth-panelStyle.GetHorizontalFrameSize())
}

func adjustPanelWidthsToTotal(widths []int, gap int, total int) []int {
	if len(widths) == 0 {
		return widths
	}
	out := append([]int{}, widths...)
	actual := 0
	for i, w := range out {
		if i > 0 {
			actual += gap
		}
		actual += renderedPanelWidth(w)
	}
	delta := total - actual
	if delta == 0 {
		return out
	}
	last := len(out) - 1
	out[last] = maxInt(1, out[last]+delta)
	return out
}

func renderedPanelWidth(requestedWidth int) int {
	if requestedWidth <= 0 {
		return 0
	}
	return lipgloss.Width(renderPanel([]string{""}, requestedWidth))
}

func (m Model) eventsPanelWidth() int {
	return m.computeLayoutSpec().usableWidth
}

func (m Model) positionsPanelWidth() int {
	return m.computeLayoutSpec().leftWidth
}

func (m Model) ordersPanelWidth() int {
	spec := m.computeLayoutSpec()
	if spec.twoColumn {
		return spec.rightWidth
	}
	return spec.usableWidth
}

func (m Model) watchlistPanelWidth() int {
	return m.computeLayoutSpec().usableWidth
}

func (m *Model) syncWidgets() {
	m.syncPositionsTable()
	m.syncOrdersTable()
	m.syncWatchlistTable()
	m.syncEventsViewport()
}

func (m *Model) syncEventsViewport() {
	width := panelInnerWidth(m.eventsPanelWidth())
	height := m.eventPageSize()
	if height < 1 {
		height = 1
	}
	m.eventsViewport.Width = width
	m.eventsViewport.Height = height
	lines := m.buildEventViewportLines()
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("(none)")}
	}
	m.eventsViewport.SetContent(strings.Join(lines, "\n"))
	m.clampEventScroll()
	m.applyEventScrollToViewport()
}

func (m Model) buildEventViewportLines() []string {
	rows := make([]string, 0, len(m.snapshot.Events))
	for _, e := range m.snapshot.Events {
		rows = append(rows, fmt.Sprintf("%s %-18s %s", formatLocalClock(e.Time), e.Type, e.Details))
	}
	return rows
}

func newPositionsTable() table.Model {
	styles := newPanelTableStyles()
	return table.New(
		table.WithRows(nil),
		table.WithColumns(positionTableColumns(40)),
		table.WithHeight(2),
		table.WithWidth(40),
		table.WithFocused(false),
		table.WithStyles(styles),
	)
}

func newOrdersTable() table.Model {
	styles := newPanelTableStyles()
	return table.New(
		table.WithRows(nil),
		table.WithColumns(orderTableColumns(40)),
		table.WithHeight(2),
		table.WithWidth(40),
		table.WithFocused(false),
		table.WithStyles(styles),
	)
}

func newWatchlistTable() table.Model {
	styles := newPanelTableStyles()
	return table.New(
		table.WithRows(nil),
		table.WithColumns(watchlistTableColumns(64)),
		table.WithHeight(2),
		table.WithWidth(64),
		table.WithFocused(false),
		table.WithStyles(styles),
	)
}

func newPanelTableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("159"))
	styles.Cell = lipgloss.NewStyle()
	styles.Selected = lipgloss.NewStyle()
	return styles
}

func (m *Model) syncPositionsTable() {
	innerWidth := panelInnerWidth(m.positionsPanelWidth())
	if innerWidth < 16 {
		return
	}
	m.positionsTable.SetWidth(innerWidth)
	m.positionsTable.SetColumns(positionTableColumns(innerWidth))
	m.positionsTable.SetRows(positionTableRows(m.snapshot.Positions))
	height := maxInt(2, len(m.snapshot.Positions)+1)
	m.positionsTable.SetHeight(height)
}

func (m *Model) syncOrdersTable() {
	innerWidth := panelInnerWidth(m.ordersPanelWidth())
	if innerWidth < 24 {
		return
	}
	m.ordersTable.SetWidth(innerWidth)
	m.ordersTable.SetColumns(orderTableColumns(innerWidth))
	m.ordersTable.SetRows(orderTableRows(m.snapshot.Orders))
	height := maxInt(2, len(m.snapshot.Orders)+1)
	m.ordersTable.SetHeight(height)
}

func (m *Model) syncWatchlistTable() {
	innerWidth := panelInnerWidth(m.watchlistPanelWidth())
	if innerWidth < 32 {
		return
	}
	m.watchlistTable.SetWidth(innerWidth)
	m.watchlistTable.SetColumns(watchlistTableColumns(innerWidth))
	m.watchlistTable.SetRows(m.watchlistTableRows())
	height := maxInt(2, len(m.watchlist)+1)
	m.watchlistTable.SetHeight(height)
}

func positionTableColumns(totalWidth int) []table.Column {
	minWidths := []int{4, 5, 5, 5}
	targetWidths := []int{8, 10, 10, 10}
	widths := fitColumnWidths(totalWidth, minWidths, targetWidths)
	return []table.Column{
		{Title: "Symbol", Width: widths[0]},
		{Title: "Qty", Width: widths[1]},
		{Title: "Avg", Width: widths[2]},
		{Title: "Last", Width: widths[3]},
	}
}

func orderTableColumns(totalWidth int) []table.Column {
	minWidths := []int{2, 9, 4, 4, 5, 6}
	targetWidths := []int{4, 9, 6, 8, 10, 12}
	widths := fitColumnWidths(totalWidth, minWidths, targetWidths)
	return []table.Column{
		{Title: "#", Width: widths[0]},
		{Title: "Order ID", Width: widths[1]},
		{Title: "Side", Width: widths[2]},
		{Title: "Symbol", Width: widths[3]},
		{Title: "Qty", Width: widths[4]},
		{Title: "Status", Width: widths[5]},
	}
}

func watchlistTableColumns(totalWidth int) []table.Column {
	minWidths := []int{4, 6, 6, 6, 5, 6, 8}
	targetWidths := []int{8, 10, 10, 10, 8, 8, 16}
	widths := fitColumnWidths(totalWidth, minWidths, targetWidths)
	return []table.Column{
		{Title: "Symbol", Width: widths[0]},
		{Title: "Last", Width: widths[1]},
		{Title: "Bid", Width: widths[2]},
		{Title: "Ask", Width: widths[3]},
		{Title: "Spr", Width: widths[4]},
		{Title: "Chg", Width: widths[5]},
		{Title: "State", Width: widths[6]},
	}
}

func fitColumnWidths(total int, minimum []int, target []int) []int {
	out := append([]int{}, minimum...)
	sum := 0
	for _, w := range out {
		sum += w
	}
	if total <= 0 || sum <= 0 {
		return out
	}
	if total < sum {
		scale := float64(total) / float64(sum)
		for i := range out {
			out[i] = maxInt(1, int(float64(out[i])*scale))
		}
		for columnWidthSum(out) > total {
			for i := len(out) - 1; i >= 0 && columnWidthSum(out) > total; i-- {
				if out[i] > 1 {
					out[i]--
				}
			}
		}
		return out
	}
	remaining := total - sum
	for i := range out {
		grow := target[i] - out[i]
		if grow <= 0 || remaining == 0 {
			continue
		}
		add := minInt(grow, remaining)
		out[i] += add
		remaining -= add
	}
	if remaining > 0 {
		out[len(out)-1] += remaining
	}
	return out
}

func columnWidthSum(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w
	}
	return total
}

func positionTableRows(positions []domain.Position) []table.Row {
	rows := make([]table.Row, 0, len(positions))
	for _, p := range positions {
		rows = append(rows, table.Row{
			runewidth.Truncate(p.Symbol, 8, ""),
			fmt.Sprintf("%.2f", p.Qty),
			fmt.Sprintf("%.2f", p.AvgCost),
			fmt.Sprintf("%.2f", p.LastPrice),
		})
	}
	return rows
}

func orderTableRows(orders []domain.Order) []table.Row {
	rows := make([]table.Row, 0, len(orders))
	for i, o := range orders {
		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			runewidth.Truncate(o.ID, 8, ""),
			strings.ToUpper(string(o.Side)),
			runewidth.Truncate(o.Symbol, 8, ""),
			fmt.Sprintf("%.2f", o.Qty),
			runewidth.Truncate(string(o.Status), 16, ""),
		})
	}
	return rows
}

func (m Model) watchlistTableRows() []table.Row {
	rows := make([]table.Row, 0, len(m.watchlist))
	for _, symbol := range m.watchlist {
		if errMsg, ok := m.quoteErr[symbol]; ok {
			rows = append(rows, table.Row{
				runewidth.Truncate(symbol, 8, ""),
				"-",
				"-",
				"-",
				"-",
				"-",
				"error: " + runewidth.Truncate(errMsg, 24, ""),
			})
			continue
		}
		q, ok := m.quotes[symbol]
		if !ok {
			rows = append(rows, table.Row{
				runewidth.Truncate(symbol, 8, ""),
				"-",
				"-",
				"-",
				"-",
				"-",
				"pending",
			})
			continue
		}
		change := "n/a"
		if prev, ok := m.prevLast[symbol]; ok && prev > 0 {
			change = formatSignedPctPlain(((q.Last - prev) / prev) * 100)
		}
		state := m.watchlistStateCell(symbol, q)
		rows = append(rows, table.Row{
			runewidth.Truncate(symbol, 8, ""),
			fmt.Sprintf("%.2f", q.Last),
			fmt.Sprintf("%.2f", q.Bid),
			fmt.Sprintf("%.2f", q.Ask),
			fmt.Sprintf("%.2f", q.Ask-q.Bid),
			change,
			state,
		})
	}
	return rows
}

func (m Model) watchlistStateCell(symbol string, quote domain.Quote) string {
	seenAt := m.quoteSeenAt[symbol]
	switch {
	case seenAt.IsZero() && quote.Time.IsZero():
		return "pending"
	case !seenAt.IsZero() && time.Since(seenAt) > watchlistStateStaleAfter:
		return "stale"
	default:
		return "ok"
	}
}

func formatSignedPctPlain(v float64) string {
	return fmt.Sprintf("%+.2f%%", v)
}

func colorizeTableColumns(view string, cols []table.Column, colorizers map[int]func(string) string) string {
	if view == "" || len(cols) == 0 || len(colorizers) == 0 {
		return view
	}
	widths := make([]int, 0, len(cols))
	for _, c := range cols {
		if c.Width > 0 {
			widths = append(widths, c.Width)
		}
	}
	if len(widths) == 0 {
		return view
	}

	lines := strings.Split(view, "\n")
	for i := 1; i < len(lines); i++ { // keep header untouched
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		segments := splitFixedWidthColumns(line, widths)
		if len(segments) == 0 {
			continue
		}
		for colIdx, colorize := range colorizers {
			if colorize == nil || colIdx < 0 || colIdx >= len(segments) {
				continue
			}
			segments[colIdx] = colorize(segments[colIdx])
		}
		lines[i] = strings.Join(segments, "")
	}
	return strings.Join(lines, "\n")
}

func splitFixedWidthColumns(line string, widths []int) []string {
	runes := []rune(line)
	segments := make([]string, 0, len(widths))
	pos := 0
	for _, w := range widths {
		if w <= 0 {
			continue
		}
		if pos >= len(runes) {
			segments = append(segments, "")
			continue
		}
		end := pos + w
		if end > len(runes) {
			end = len(runes)
		}
		segments = append(segments, string(runes[pos:end]))
		pos = end
	}
	return segments
}
