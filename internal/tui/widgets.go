package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

func (m Model) systemRuntimePanelWidth() int {
	return m.computeLayoutSpec().leftWidth
}

func (m Model) systemAgentPanelWidth() int {
	spec := m.computeLayoutSpec()
	if spec.twoColumn {
		return spec.rightWidth
	}
	return spec.usableWidth
}

func (m Model) systemPersistencePanelWidth() int {
	return m.computeLayoutSpec().usableWidth
}

func (m Model) strategyRecommendationsPanelWidth() int {
	return m.computeLayoutSpec().usableWidth
}

func (m *Model) syncWidgets() {
	m.syncPositionsTable()
	m.syncOrdersTable()
	m.syncWatchlistTable()
	m.syncSystemTables()
	m.syncEventsViewport()
	m.syncStrategyViewport()
	m.syncHelpModel()
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
	m.eventLines = append(m.eventLines[:0], lines...)
	m.eventLinesReady = true
	m.eventLinesEvents = len(m.snapshot.Events)
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("(none)")}
	}
	m.eventsViewport.SetContent(strings.Join(lines, "\n"))
	m.clampEventScroll()
	m.applyEventScrollToViewport()
}

func (m Model) buildEventViewportLines() []string {
	events := make([]domain.Event, 0, len(m.snapshot.Events))
	for _, e := range m.snapshot.Events {
		if includeEventInLogs(e.Type) {
			events = append(events, e)
		}
	}
	rows := make([]string, 0, len(events))
	maxWidth := maxInt(1, m.eventsViewport.Width)
	typeWidth := eventTypeColumnWidth(events, maxWidth)
	for _, e := range events {
		rows = append(rows, renderEventRows(e, typeWidth, maxWidth)...)
	}
	return rows
}

func includeEventInLogs(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "event_persist_stats":
		return false
	default:
		return true
	}
}

func eventTypeColumnWidth(events []domain.Event, panelWidth int) int {
	minWidth := 14
	maxWidth := 28
	best := minWidth
	for _, e := range events {
		w := runewidth.StringWidth(strings.TrimSpace(e.Type))
		if w > best {
			best = w
		}
	}
	best = minInt(best, maxWidth)
	// Keep enough room for details.
	maxByPanel := maxInt(minWidth, panelWidth-(8+1+minWidth+1+20))
	return minInt(best, maxByPanel)
}

func renderEventRows(event domain.Event, typeWidth int, panelWidth int) []string {
	timeText := formatLocalClock(event.Time)
	typeText := strings.TrimSpace(event.Type)
	if typeText == "" {
		typeText = "-"
	}
	typeText = runewidth.Truncate(typeText, typeWidth, "…")
	prefixPlain := fmt.Sprintf("%-8s %-*s ", timeText, typeWidth, typeText)
	detailWidth := maxInt(12, panelWidth-runewidth.StringWidth(prefixPlain))
	detailLines := renderDetailWrappedLines(event.Details, detailWidth)
	if len(detailLines) == 0 {
		detailLines = []renderedDetailLine{{plain: "-", styled: mutedStyle.Render("-")}}
	}

	typeStyled := eventTypeStyle(event.Type).Render(typeText)
	prefixStyled := fmt.Sprintf("%-8s ", mutedStyle.Render(timeText)) + lipgloss.NewStyle().Width(typeWidth).MaxWidth(typeWidth).Render(typeStyled) + " "
	contPrefixStyled := strings.Repeat(" ", runewidth.StringWidth(prefixPlain))

	out := make([]string, 0, len(detailLines))
	for i, d := range detailLines {
		if i == 0 {
			out = append(out, fitDisplayWidth(prefixStyled+d.styled, panelWidth))
			continue
		}
		if d.plain == "" {
			out = append(out, fitDisplayWidth(contPrefixStyled, panelWidth))
			continue
		}
		out = append(out, fitDisplayWidth(contPrefixStyled+d.styled, panelWidth))
	}
	return out
}

func fitDisplayWidth(s string, width int) string {
	_ = width
	return s
}

type renderedDetailLine struct {
	plain  string
	styled string
}

type detailToken struct {
	plain  string
	styled string
	width  int
}

func renderDetailWrappedLines(details string, width int) []renderedDetailLine {
	width = maxInt(1, width)
	details = strings.ReplaceAll(details, "\r\n", "\n")
	if strings.TrimSpace(details) == "" {
		return []renderedDetailLine{{plain: "-", styled: mutedStyle.Render("-")}}
	}

	physical := strings.Split(details, "\n")
	out := make([]renderedDetailLine, 0, len(physical))
	for _, line := range physical {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			out = append(out, renderedDetailLine{plain: "", styled: ""})
			continue
		}
		tokens := tokenizeDetailLine(line)
		wrapped := wrapDetailTokens(tokens, width)
		out = append(out, wrapped...)
	}
	return out
}

func tokenizeDetailLine(line string) []detailToken {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	out := make([]detailToken, 0, len(fields))
	for _, tok := range fields {
		styled := styleDetailToken(tok)
		out = append(out, detailToken{
			plain:  tok,
			styled: styled,
			width:  runewidth.StringWidth(tok),
		})
	}
	return out
}

func wrapDetailTokens(tokens []detailToken, width int) []renderedDetailLine {
	width = maxInt(1, width)
	if len(tokens) == 0 {
		return []renderedDetailLine{{plain: "", styled: ""}}
	}
	out := make([]renderedDetailLine, 0, 2)
	currentPlain := make([]string, 0, len(tokens))
	currentStyled := make([]string, 0, len(tokens))
	currentWidth := 0
	flush := func() {
		if len(currentPlain) == 0 {
			return
		}
		out = append(out, renderedDetailLine{
			plain:  strings.Join(currentPlain, " "),
			styled: strings.Join(currentStyled, " "),
		})
		currentPlain = currentPlain[:0]
		currentStyled = currentStyled[:0]
		currentWidth = 0
	}

	for _, tok := range tokens {
		tokWidth := maxInt(1, tok.width)
		sep := 0
		if len(currentPlain) > 0 {
			sep = 1
		}
		if len(currentPlain) > 0 && currentWidth+sep+tokWidth <= width {
			currentPlain = append(currentPlain, tok.plain)
			currentStyled = append(currentStyled, tok.styled)
			currentWidth += sep + tokWidth
			continue
		}
		if len(currentPlain) == 0 && tokWidth <= width {
			currentPlain = append(currentPlain, tok.plain)
			currentStyled = append(currentStyled, tok.styled)
			currentWidth = tokWidth
			continue
		}
		if len(currentPlain) > 0 {
			flush()
		}
		if tokWidth <= width {
			currentPlain = append(currentPlain, tok.plain)
			currentStyled = append(currentStyled, tok.styled)
			currentWidth = tokWidth
			continue
		}
		// Very long token: hard-wrap by display width.
		parts := splitTokenByDisplayWidth(tok.plain, width)
		for _, part := range parts {
			out = append(out, renderedDetailLine{
				plain:  part,
				styled: part,
			})
		}
	}
	flush()
	if len(out) == 0 {
		return []renderedDetailLine{{plain: "", styled: ""}}
	}
	return out
}

func splitTokenByDisplayWidth(token string, width int) []string {
	width = maxInt(1, width)
	remaining := token
	out := make([]string, 0, 2)
	for runewidth.StringWidth(remaining) > width {
		part := firstDisplayWidthSegment(remaining, width)
		if part == "" {
			break
		}
		out = append(out, part)
		remaining = remaining[len(part):]
	}
	if remaining != "" {
		out = append(out, remaining)
	}
	if len(out) == 0 {
		return []string{token}
	}
	return out
}

func firstDisplayWidthSegment(s string, maxWidth int) string {
	maxWidth = maxInt(1, maxWidth)
	width := 0
	cut := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if rw <= 0 {
			rw = 1
		}
		if width+rw > maxWidth {
			break
		}
		width += rw
		cut = i + len(string(r))
	}
	if cut == 0 {
		_, size := utf8.DecodeRuneInString(s)
		if size <= 0 {
			return s
		}
		return s[:size]
	}
	return s[:cut]
}

func eventTypeStyle(eventType string) lipgloss.Style {
	t := strings.ToLower(strings.TrimSpace(eventType))
	switch {
	case strings.Contains(t, "error"), strings.Contains(t, "rejected"), strings.Contains(t, "failed"), strings.Contains(t, "panic"), strings.Contains(t, "drift_detected"):
		return errStyle
	case strings.Contains(t, "warn"), strings.Contains(t, "skipped"), strings.Contains(t, "idle"), strings.Contains(t, "canceled"):
		return warnStyle
	case strings.Contains(t, "executed"), strings.Contains(t, "placed"), strings.Contains(t, "created"), strings.Contains(t, "approved"):
		return positiveStyle
	default:
		return headerValueStyle
	}
}

func styleDetailToken(token string) string {
	key, value, ok := strings.Cut(token, "=")
	if !ok {
		return token
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" {
		return token
	}
	keyStyled := headerLabelStyle.Render(key)
	valueStyled := styleDetailValue(key, value)
	return keyStyled + "=" + valueStyled
}

func styleDetailValue(key string, value string) string {
	lk := strings.ToLower(strings.TrimSpace(key))
	lv := strings.ToLower(strings.TrimSpace(strings.Trim(value, "\"")))
	switch lv {
	case "true", "yes", "ok", "active", "open":
		return positiveStyle.Render(value)
	case "false", "no", "none", "n/a", "idle", "clear":
		return mutedStyle.Render(value)
	case "error", "failed", "rejected":
		return errStyle.Render(value)
	}
	switch lk {
	case "reason", "error", "rejection_reason":
		return warnStyle.Render(value)
	case "state", "status":
		if lv == "ok" || lv == "filled" || lv == "active" {
			return positiveStyle.Render(value)
		}
		if lv == "error" || lv == "failed" || lv == "rejected" {
			return errStyle.Render(value)
		}
		return warnStyle.Render(value)
	case "pdt", "avoid_pdt", "avoid_gfv", "drift_detected":
		if lv == "true" {
			return warnStyle.Render(value)
		}
		return mutedStyle.Render(value)
	}
	if numericLike(value) {
		if strings.HasPrefix(strings.TrimSpace(value), "-") {
			return negativeStyle.Render(value)
		}
		if strings.HasPrefix(strings.TrimSpace(value), "+") {
			return positiveStyle.Render(value)
		}
	}
	return headerValueStyle.Render(value)
}

func numericLike(raw string) bool {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "%")
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func (m *Model) syncStrategyViewport() {
	width := panelInnerWidth(m.strategyRecommendationsPanelWidth())
	height := m.strategyPageSize()
	if height < 1 {
		height = 1
	}
	m.strategyViewport.Width = width
	m.strategyViewport.Height = height
	lines := m.buildStrategyRecommendationBodyRows()
	if len(lines) == 0 {
		lines = []string{mutedStyle.Render("(none)")}
	}
	m.strategyViewport.SetContent(strings.Join(lines, "\n"))
}

func (m *Model) syncHelpModel() {
	m.helpModel.Width = maxInt(1, m.computeLayoutSpec().usableWidth)
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

func newSystemTable() table.Model {
	styles := newPanelTableStyles()
	return table.New(
		table.WithRows(nil),
		table.WithColumns(systemTableColumns(48)),
		table.WithHeight(2),
		table.WithWidth(48),
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
	m.positionsTable.SetRows(positionTableRows(m.snapshot.Positions, m.quotes))
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

func (m *Model) syncSystemTables() {
	m.syncSystemTable(&m.systemRuntimeTable, m.systemRuntimePanelWidth(), systemKVRows(m.systemRuntimeData()))
	m.syncSystemTable(&m.systemAgentTable, m.systemAgentPanelWidth(), systemKVRows(m.systemAgentData()))
	m.syncSystemTable(&m.systemPersistTable, m.systemPersistencePanelWidth(), systemKVRows(m.systemPersistenceData()))
}

func (m *Model) syncSystemTable(tbl *table.Model, panelWidth int, rows []table.Row) {
	innerWidth := panelInnerWidth(panelWidth)
	if innerWidth < 24 {
		return
	}
	tbl.SetWidth(innerWidth)
	tbl.SetColumns(systemTableColumns(innerWidth))
	tbl.SetRows(rows)
	height := maxInt(2, len(rows)+1)
	tbl.SetHeight(height)
}

func positionTableColumns(totalWidth int) []table.Column {
	minWidths := []int{4, 4, 4, 4, 6}
	targetWidths := []int{8, 10, 8, 8, 11}
	widths := fitTableColumnWidths(totalWidth, minWidths, targetWidths)
	return []table.Column{
		{Title: "Symbol", Width: widths[0]},
		{Title: "Qty", Width: widths[1]},
		{Title: "Avg", Width: widths[2]},
		{Title: "Last", Width: widths[3]},
		{Title: "uPnL", Width: widths[4]},
	}
}

func orderTableColumns(totalWidth int) []table.Column {
	minWidths := []int{2, 9, 4, 4, 5, 5, 6}
	targetWidths := []int{4, 10, 6, 7, 8, 8, 10}
	widths := fitTableColumnWidths(totalWidth, minWidths, targetWidths)
	return []table.Column{
		{Title: "#", Width: widths[0]},
		{Title: "Order ID", Width: widths[1]},
		{Title: "Side", Width: widths[2]},
		{Title: "Symbol", Width: widths[3]},
		{Title: "Qty", Width: widths[4]},
		{Title: "Limit", Width: widths[5]},
		{Title: "Status", Width: widths[6]},
	}
}

func systemTableColumns(totalWidth int) []table.Column {
	minWidths := []int{8, 12}
	targetWidths := []int{16, 34}
	widths := fitTableColumnWidths(totalWidth, minWidths, targetWidths)
	return []table.Column{
		{Title: "Key", Width: widths[0]},
		{Title: "Value", Width: widths[1]},
	}
}

func watchlistTableColumns(totalWidth int) []table.Column {
	minWidths := []int{4, 6, 6, 6, 5, 6, 8}
	targetWidths := []int{10, 10, 10, 10, 10, 10, 18}
	widths := fitTableColumnWidths(totalWidth, minWidths, targetWidths)
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

func fitTableColumnWidths(total int, minimum []int, target []int) []int {
	if len(minimum) == 0 {
		return nil
	}
	separatorBudget := len(minimum) - 1
	contentWidth := total - separatorBudget
	if contentWidth < len(minimum) {
		contentWidth = len(minimum)
	}
	return fitColumnWidths(contentWidth, minimum, target)
}

func columnWidthSum(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w
	}
	return total
}

func positionTableRows(positions []domain.Position, quotes map[string]domain.Quote) []table.Row {
	rows := make([]table.Row, 0, len(positions))
	for _, p := range positions {
		mark := p.LastPrice
		if q, ok := quotes[p.Symbol]; ok && q.Last > 0 {
			mark = q.Last
		}
		if mark <= 0 {
			mark = p.AvgCost
		}
		upnl := (mark - p.AvgCost) * p.Qty
		rows = append(rows, table.Row{
			runewidth.Truncate(p.Symbol, 8, ""),
			fmt.Sprintf("%.2f", p.Qty),
			fmt.Sprintf("%.2f", p.AvgCost),
			fmt.Sprintf("%.2f", mark),
			fmt.Sprintf("%+.2f", upnl),
		})
	}
	return rows
}

func orderTableRows(orders []domain.Order) []table.Row {
	rows := make([]table.Row, 0, len(orders))
	for i, o := range orders {
		limit := "-"
		if o.LimitPrice != nil && *o.LimitPrice > 0 {
			limit = fmt.Sprintf("%.2f", *o.LimitPrice)
		}
		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			runewidth.Truncate(o.ID, 8, ""),
			strings.ToUpper(string(o.Side)),
			runewidth.Truncate(o.Symbol, 8, ""),
			fmt.Sprintf("%.2f", o.Qty),
			limit,
			runewidth.Truncate(string(o.Status), 16, ""),
		})
	}
	return rows
}

func systemKVRows(items []systemKV) []table.Row {
	rows := make([]table.Row, 0, len(items))
	for _, item := range items {
		rows = append(rows, table.Row{item.key, item.value})
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
	for idx, w := range widths {
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
		segment := string(runes[pos:end])
		pos = end
		// Bubble tables separate columns with spaces; keep one separator with the cell.
		if idx < len(widths)-1 && pos < len(runes) && runes[pos] == ' ' {
			segment += string(runes[pos])
			pos++
		}
		segments = append(segments, segment)
	}
	return segments
}
