package alpaca

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	sdkalpaca "github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/shopspring/decimal"

	"helix-tui/internal/domain"
	"helix-tui/internal/symbols"
)

const (
	PaperAPIBase = "https://paper-api.alpaca.markets"
	LiveAPIBase  = "https://api.alpaca.markets"
	DataAPIBase  = "https://data.alpaca.markets"
	DataWSBase   = "https://stream.data.alpaca.markets/v2"
	EnvPaper     = "paper"
	EnvLive      = "live"
	// Guard against pathological NBBO outliers that can produce unrealistic mid prices.
	maxQuoteSpreadPctForMid = 25.0
)

type Broker struct {
	tradeClient *sdkalpaca.Client
	dataClient  *marketdata.Client
	feed        marketdata.Feed
	apiKey      string
	apiSecret   string
	streamBase  string
}

type silentStreamLogger struct{}

func (silentStreamLogger) Infof(string, ...interface{})  {}
func (silentStreamLogger) Warnf(string, ...interface{})  {}
func (silentStreamLogger) Errorf(string, ...interface{}) {}

type Config struct {
	BaseURL     string
	DataBaseURL string
	APIKey      string
	APISecret   string
	Feed        string
}

func New(cfg Config) *Broker {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = LiveAPIBase
	}
	dataBaseURL := strings.TrimSpace(cfg.DataBaseURL)
	if dataBaseURL == "" {
		dataBaseURL = strings.TrimSpace(os.Getenv("APCA_API_DATA_URL"))
	}
	if dataBaseURL == "" {
		dataBaseURL = DataAPIBase
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	apiSecret := strings.TrimSpace(cfg.APISecret)
	feed := normalizeFeed(cfg.Feed)

	return &Broker{
		tradeClient: sdkalpaca.NewClient(sdkalpaca.ClientOpts{
			BaseURL:   baseURL,
			APIKey:    apiKey,
			APISecret: apiSecret,
		}),
		dataClient: marketdata.NewClient(marketdata.ClientOpts{
			BaseURL:   dataBaseURL,
			APIKey:    apiKey,
			APISecret: apiSecret,
			Feed:      feed,
		}),
		feed:       feed,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		streamBase: stockStreamBaseURL(dataBaseURL),
	}
}

func NewForEnv(env, apiKey, secret, baseURL, dataBaseURL, feed string) *Broker {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = BaseURLForEnv(env)
	}
	return New(Config{
		BaseURL:     baseURL,
		DataBaseURL: dataBaseURL,
		APIKey:      apiKey,
		APISecret:   secret,
		Feed:        feed,
	})
}

func NewPaper(apiKey, secret, dataBaseURL, feed string) *Broker {
	return NewForEnv(EnvPaper, apiKey, secret, "", dataBaseURL, feed)
}

func BaseURLForEnv(env string) string {
	switch NormalizeEnv(env) {
	case EnvLive:
		return LiveAPIBase
	default:
		return PaperAPIBase
	}
}

func NormalizeEnv(env string) string {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case EnvLive:
		return EnvLive
	case EnvPaper, "":
		return EnvPaper
	default:
		return EnvPaper
	}
}

func (b *Broker) GetAccount(ctx context.Context) (domain.Account, error) {
	if err := ctx.Err(); err != nil {
		return domain.Account{}, err
	}

	account, err := b.tradeClient.GetAccount()
	if err != nil {
		return domain.Account{}, err
	}
	return domain.Account{
		Cash:             account.Cash.InexactFloat64(),
		BuyingPower:      account.BuyingPower.InexactFloat64(),
		Equity:           account.Equity.InexactFloat64(),
		Multiplier:       account.Multiplier.InexactFloat64(),
		PatternDayTrader: account.PatternDayTrader,
		DayTradeCount:    int(account.DaytradeCount),
	}, nil
}

func (b *Broker) GetPositions(ctx context.Context) ([]domain.Position, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	positions, err := b.tradeClient.GetPositions()
	if err != nil {
		return nil, err
	}

	out := make([]domain.Position, 0, len(positions))
	for _, p := range positions {
		lastPrice := 0.0
		if p.CurrentPrice != nil {
			lastPrice = p.CurrentPrice.InexactFloat64()
		}
		out = append(out, domain.Position{
			Symbol:    p.Symbol,
			Qty:       p.Qty.InexactFloat64(),
			AvgCost:   p.AvgEntryPrice.InexactFloat64(),
			LastPrice: lastPrice,
		})
	}
	return out, nil
}

func (b *Broker) GetOpenOrders(ctx context.Context) ([]domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	orders, err := b.tradeClient.GetOrders(sdkalpaca.GetOrdersRequest{
		Status: "open",
	})
	if err != nil {
		return nil, err
	}

	out := make([]domain.Order, 0, len(orders))
	for _, o := range orders {
		out = append(out, toDomainOrder(o))
	}
	return out, nil
}

func (b *Broker) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	if err := ctx.Err(); err != nil {
		return domain.Quote{}, err
	}

	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return domain.Quote{}, fmt.Errorf("symbol is required")
	}

	quoteReq := marketdata.GetLatestQuoteRequest{}
	if b.feed != "" {
		quoteReq.Feed = b.feed
	}
	quote, err := b.dataClient.GetLatestQuote(symbol, quoteReq)
	if err != nil {
		return domain.Quote{}, err
	}
	if quote == nil {
		return domain.Quote{}, fmt.Errorf("no quote returned for %s", symbol)
	}

	return domain.Quote{
		Symbol: symbol,
		Bid:    quote.BidPrice,
		Ask:    quote.AskPrice,
		Last:   quoteLastFromBidAsk(quote.BidPrice, quote.AskPrice),
		Time:   quote.Timestamp,
	}, nil
}

func (b *Broker) PlaceOrder(ctx context.Context, req domain.OrderRequest) (domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return domain.Order{}, err
	}
	if req.Qty <= 0 {
		return domain.Order{}, fmt.Errorf("qty must be greater than 0")
	}

	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return domain.Order{}, fmt.Errorf("symbol is required")
	}

	qty := decimal.NewFromFloat(req.Qty)
	orderType := toSDKOrderType(req.Type)
	orderReq := sdkalpaca.PlaceOrderRequest{
		Symbol:        symbol,
		Qty:           &qty,
		Side:          toSDKSide(req.Side),
		Type:          orderType,
		TimeInForce:   sdkalpaca.Day,
		ClientOrderID: req.ClientOrderID,
	}
	if req.Type == domain.OrderTypeLimit {
		if req.LimitPrice == nil || *req.LimitPrice <= 0 {
			return domain.Order{}, fmt.Errorf("limit price must be > 0")
		}
		limit := decimal.NewFromFloat(*req.LimitPrice)
		orderReq.LimitPrice = &limit
	}

	order, err := b.tradeClient.PlaceOrder(orderReq)
	if err != nil {
		return domain.Order{}, err
	}
	return toDomainOrder(*order), nil
}

func (b *Broker) CancelOrder(ctx context.Context, orderID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(orderID) == "" {
		return fmt.Errorf("order id is required")
	}
	return b.tradeClient.CancelOrder(orderID)
}

func (b *Broker) StreamTradeUpdates(ctx context.Context) (<-chan domain.TradeUpdate, error) {
	out := make(chan domain.TradeUpdate, 128)
	go func() {
		defer close(out)
		_ = b.tradeClient.StreamTradeUpdates(ctx, func(update sdkalpaca.TradeUpdate) {
			select {
			case <-ctx.Done():
				return
			case out <- toDomainTradeUpdate(update):
			}
		}, sdkalpaca.StreamTradeUpdatesRequest{})
	}()
	return out, nil
}

func (b *Broker) StreamQuotes(ctx context.Context, symbols []string) (<-chan domain.Quote, <-chan error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	normalized := normalizeSymbols(symbols)
	quotes := make(chan domain.Quote, 1024)
	errs := make(chan error, 16)
	if len(normalized) == 0 {
		close(quotes)
		close(errs)
		return quotes, errs, nil
	}

	client := stream.NewStocksClient(
		b.feed,
		stream.WithLogger(silentStreamLogger{}),
		stream.WithCredentials(b.apiKey, b.apiSecret),
		stream.WithBaseURL(b.streamBase),
		stream.WithReconnectSettings(0, 250*time.Millisecond),
		stream.WithQuotes(func(q stream.Quote) {
			dq := toDomainStreamQuote(q)
			select {
			case <-ctx.Done():
				return
			case quotes <- dq:
			default:
			}
		}, normalized...),
	)

	go func() {
		defer close(quotes)
		defer close(errs)
		if err := client.Connect(ctx); err != nil {
			if ctx.Err() == nil {
				errs <- fmt.Errorf("connect quote stream: %w", err)
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case err, ok := <-client.Terminated():
			if !ok || err == nil || ctx.Err() != nil {
				return
			}
			errs <- fmt.Errorf("quote stream terminated: %w", err)
		}
	}()
	return quotes, errs, nil
}

func (b *Broker) GetWatchlistSymbols(name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("watchlist name is required")
	}
	watchlists, err := b.tradeClient.GetWatchlists()
	if err != nil {
		return nil, err
	}
	for _, wl := range watchlists {
		if !strings.EqualFold(strings.TrimSpace(wl.Name), name) {
			continue
		}
		full, err := b.tradeClient.GetWatchlist(wl.ID)
		if err != nil {
			return nil, err
		}
		return normalizeSymbolsFromAssets(full.Assets), nil
	}
	return nil, nil
}

func (b *Broker) UpsertWatchlistSymbols(name string, symbols []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("watchlist name is required")
	}
	symbols = normalizeSymbols(symbols)
	watchlists, err := b.tradeClient.GetWatchlists()
	if err != nil {
		return err
	}
	for _, wl := range watchlists {
		if !strings.EqualFold(strings.TrimSpace(wl.Name), name) {
			continue
		}
		_, err := b.tradeClient.UpdateWatchlist(wl.ID, sdkalpaca.UpdateWatchlistRequest{
			Name:    name,
			Symbols: symbols,
		})
		return err
	}
	_, err = b.tradeClient.CreateWatchlist(sdkalpaca.CreateWatchlistRequest{
		Name:    name,
		Symbols: symbols,
	})
	return err
}

func toDomainOrder(o sdkalpaca.Order) domain.Order {
	return domain.Order{
		ID:         o.ID,
		Symbol:     o.Symbol,
		Side:       toDomainSide(o.Side),
		Qty:        decimalPtrToFloat(o.Qty),
		FilledQty:  o.FilledQty.InexactFloat64(),
		Type:       toDomainOrderType(o.Type),
		LimitPrice: decimalPtrToFloatPtr(o.LimitPrice),
		Status:     toDomainOrderStatus(o.Status),
		CreatedAt:  o.CreatedAt,
		UpdatedAt:  o.UpdatedAt,
	}
}

func toDomainTradeUpdate(u sdkalpaca.TradeUpdate) domain.TradeUpdate {
	fillPrice := decimalPtrToFloatPtr(u.Price)
	if fillPrice == nil {
		fillPrice = decimalPtrToFloatPtr(u.Order.FilledAvgPrice)
	}
	return domain.TradeUpdate{
		OrderID:   u.Order.ID,
		Status:    toDomainOrderStatus(u.Order.Status),
		FillQty:   u.Order.FilledQty.InexactFloat64(),
		FillPrice: fillPrice,
		Time:      nonZeroTime(u.At, time.Now().UTC()),
	}
}

func toDomainStreamQuote(q stream.Quote) domain.Quote {
	return domain.Quote{
		Symbol: strings.ToUpper(strings.TrimSpace(q.Symbol)),
		Bid:    q.BidPrice,
		Ask:    q.AskPrice,
		Last:   quoteLastFromBidAsk(q.BidPrice, q.AskPrice),
		Time:   q.Timestamp,
	}
}

func quoteLastFromBidAsk(bid, ask float64) float64 {
	if bid > 0 && ask > 0 {
		if ask < bid {
			return bid
		}
		spread := ask - bid
		if bid > 0 {
			spreadPct := (spread / bid) * 100
			if spreadPct > maxQuoteSpreadPctForMid {
				return bid
			}
		}
		return bid + spread/2
	}
	if bid > 0 {
		return bid
	}
	if ask > 0 {
		return ask
	}
	return 0
}

func toSDKSide(side domain.Side) sdkalpaca.Side {
	switch side {
	case domain.SideSell:
		return sdkalpaca.Sell
	case domain.SideBuy:
		return sdkalpaca.Buy
	default:
		return sdkalpaca.Buy
	}
}

func toSDKOrderType(t domain.OrderType) sdkalpaca.OrderType {
	switch t {
	case domain.OrderTypeLimit:
		return sdkalpaca.Limit
	case domain.OrderTypeMarket:
		return sdkalpaca.Market
	default:
		return sdkalpaca.Market
	}
}

func toDomainSide(side sdkalpaca.Side) domain.Side {
	switch side {
	case sdkalpaca.Sell:
		return domain.SideSell
	default:
		return domain.SideBuy
	}
}

func toDomainOrderType(t sdkalpaca.OrderType) domain.OrderType {
	switch t {
	case sdkalpaca.Limit:
		return domain.OrderTypeLimit
	default:
		return domain.OrderTypeMarket
	}
}

func toDomainOrderStatus(s string) domain.OrderStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "new":
		return domain.OrderStatusNew
	case "accepted", "pending_new", "accepted_for_bidding", "pending_replace", "calculated":
		return domain.OrderStatusAccepted
	case "partially_filled":
		return domain.OrderStatusPartially
	case "filled":
		return domain.OrderStatusFilled
	case "canceled", "cancelled":
		return domain.OrderStatusCanceled
	case "rejected", "stopped", "suspended":
		return domain.OrderStatusRejected
	default:
		return domain.OrderStatus(s)
	}
}

func decimalPtrToFloat(v *decimal.Decimal) float64 {
	if v == nil {
		return 0
	}
	return v.InexactFloat64()
}

func decimalPtrToFloatPtr(v *decimal.Decimal) *float64 {
	if v == nil {
		return nil
	}
	f := v.InexactFloat64()
	return &f
}

func nonZeroTime(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}

func normalizeFeed(raw string) marketdata.Feed {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return marketdata.IEX
	case marketdata.SIP:
		return marketdata.SIP
	case marketdata.IEX:
		return marketdata.IEX
	case marketdata.DelayedSIP:
		return marketdata.DelayedSIP
	case marketdata.BOATS:
		return marketdata.BOATS
	case marketdata.Overnight:
		return marketdata.Overnight
	default:
		return marketdata.IEX
	}
}

func stockStreamBaseURL(dataBaseURL string) string {
	dataBaseURL = strings.TrimSpace(dataBaseURL)
	if dataBaseURL == "" {
		return DataWSBase
	}
	parsed, err := url.Parse(dataBaseURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return DataWSBase
	}
	host := parsed.Host
	lowerHost := strings.ToLower(host)
	switch {
	case strings.HasPrefix(lowerHost, "stream.data."):
	case strings.HasPrefix(lowerHost, "data."):
		host = "stream." + host
	default:
	}
	parsed.Host = host
	parsed.Path = "/v2"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeSymbols(raw []string) []string {
	return symbols.Normalize(raw)
}

func normalizeSymbolsFromAssets(assets []sdkalpaca.Asset) []string {
	raw := make([]string, 0, len(assets))
	for _, asset := range assets {
		raw = append(raw, asset.Symbol)
	}
	return symbols.Normalize(raw)
}
