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
	styles.Cell = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
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
	minWidths := []int{2, 8, 4, 4, 5, 6}
	targetWidths := []int{4, 16, 6, 8, 10, 12}
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
		last := fmt.Sprintf("%.2f", p.LastPrice)
		switch {
		case p.AvgCost > 0 && p.LastPrice > p.AvgCost:
			last = positiveStyle.Render(last)
		case p.AvgCost > 0 && p.LastPrice < p.AvgCost:
			last = negativeStyle.Render(last)
		default:
			last = mutedStyle.Render(last)
		}
		rows = append(rows, table.Row{
			runewidth.Truncate(p.Symbol, 8, ""),
			fmt.Sprintf("%.2f", p.Qty),
			fmt.Sprintf("%.2f", p.AvgCost),
			last,
		})
	}
	return rows
}

func orderTableRows(orders []domain.Order) []table.Row {
	rows := make([]table.Row, 0, len(orders))
	for i, o := range orders {
		side := strings.ToUpper(string(o.Side))
		if o.Side == domain.SideBuy {
			side = positiveStyle.Render(side)
		} else if o.Side == domain.SideSell {
			side = negativeStyle.Render(side)
		}

		statusText := runewidth.Truncate(string(o.Status), 16, "")
		switch o.Status {
		case domain.OrderStatusFilled:
			statusText = positiveStyle.Render(statusText)
		case domain.OrderStatusRejected, domain.OrderStatusCanceled:
			statusText = warnStyle.Render(statusText)
		case domain.OrderStatusPartially:
			statusText = lipgloss.NewStyle().Foreground(lipgloss.Color("111")).Render(statusText)
		default:
			statusText = mutedStyle.Render(statusText)
		}

		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			runewidth.Truncate(o.ID, 16, ""),
			side,
			runewidth.Truncate(o.Symbol, 8, ""),
			fmt.Sprintf("%.2f", o.Qty),
			statusText,
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
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				errStyle.Render("error: " + runewidth.Truncate(errMsg, 24, "")),
			})
			continue
		}
		q, ok := m.quotes[symbol]
		if !ok {
			rows = append(rows, table.Row{
				runewidth.Truncate(symbol, 8, ""),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("-"),
				mutedStyle.Render("pending"),
			})
			continue
		}
		change := mutedStyle.Render("n/a")
		if prev, ok := m.prevLast[symbol]; ok && prev > 0 {
			changePct := ((q.Last - prev) / prev) * 100
			raw := formatSignedPctPlain(changePct)
			switch {
			case changePct > 0:
				change = positiveStyle.Render(raw)
			case changePct < 0:
				change = negativeStyle.Render(raw)
			default:
				change = mutedStyle.Render(raw)
			}
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
		return mutedStyle.Render("pending")
	case !seenAt.IsZero() && time.Since(seenAt) > watchlistStateStaleAfter:
		return warnStyle.Render("stale")
	default:
		return okStyle.Render("ok")
	}
}

func formatSignedPctPlain(v float64) string {
	return fmt.Sprintf("%+.2f%%", v)
}
