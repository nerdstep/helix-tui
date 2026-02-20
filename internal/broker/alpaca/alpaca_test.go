package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	sdkalpaca "github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata/stream"
	"github.com/shopspring/decimal"

	"helix-tui/internal/domain"
)

func TestNormalizeFeed(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want marketdata.Feed
	}{
		{name: "default empty", in: "", want: marketdata.IEX},
		{name: "iex", in: "iex", want: marketdata.IEX},
		{name: "sip uppercase", in: "SIP", want: marketdata.SIP},
		{name: "delayed sip", in: "delayed_sip", want: marketdata.DelayedSIP},
		{name: "boats", in: "boats", want: marketdata.BOATS},
		{name: "overnight", in: "overnight", want: marketdata.Overnight},
		{name: "unknown defaults", in: "nope", want: marketdata.IEX},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFeed(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeFeed(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStockStreamBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "default", in: "", want: DataWSBase},
		{name: "standard data host", in: "https://data.alpaca.markets", want: "https://stream.data.alpaca.markets/v2"},
		{name: "already stream host", in: "https://stream.data.alpaca.markets/v2", want: "https://stream.data.alpaca.markets/v2"},
		{name: "custom host", in: "https://proxy.local", want: "https://proxy.local/v2"},
		{name: "invalid", in: "://bad", want: DataWSBase},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stockStreamBaseURL(tt.in)
			if got != tt.want {
				t.Fatalf("stockStreamBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNew_DefaultsAndPaper(t *testing.T) {
	t.Setenv("APCA_API_DATA_URL", "https://env-data.example")
	b := New(Config{})
	if b == nil || b.tradeClient == nil || b.dataClient == nil {
		t.Fatalf("expected initialized broker clients")
	}
	if b.feed != marketdata.IEX {
		t.Fatalf("expected default IEX feed, got %q", b.feed)
	}

	p := NewPaper("k", "s", "", "sip")
	if p == nil || p.tradeClient == nil || p.dataClient == nil {
		t.Fatalf("expected paper broker clients")
	}
	if p.feed != marketdata.SIP {
		t.Fatalf("expected SIP feed, got %q", p.feed)
	}

	l := NewForEnv("live", "k", "s", "", "", "iex")
	if l == nil || l.tradeClient == nil || l.dataClient == nil {
		t.Fatalf("expected live broker clients")
	}
}

func TestNormalizeEnvAndBaseURLForEnv(t *testing.T) {
	if got := NormalizeEnv(""); got != EnvPaper {
		t.Fatalf("expected default env paper, got %q", got)
	}
	if got := NormalizeEnv("LIVE"); got != EnvLive {
		t.Fatalf("expected normalized env live, got %q", got)
	}
	if got := NormalizeEnv("invalid"); got != EnvPaper {
		t.Fatalf("expected invalid env to fall back to paper, got %q", got)
	}

	if got := BaseURLForEnv("paper"); got != PaperAPIBase {
		t.Fatalf("unexpected paper base URL: %q", got)
	}
	if got := BaseURLForEnv("live"); got != LiveAPIBase {
		t.Fatalf("unexpected live base URL: %q", got)
	}
}

func TestContextAndValidationGuards(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	b := &Broker{}

	if _, err := b.GetAccount(canceled); err == nil {
		t.Fatalf("expected context error")
	}
	if _, err := b.GetPositions(canceled); err == nil {
		t.Fatalf("expected context error")
	}
	if _, err := b.GetOpenOrders(canceled); err == nil {
		t.Fatalf("expected context error")
	}
	if _, err := b.GetQuote(canceled, "AAPL"); err == nil {
		t.Fatalf("expected context error")
	}
	if _, err := b.PlaceOrder(canceled, domain.OrderRequest{}); err == nil {
		t.Fatalf("expected context error")
	}
	if err := b.CancelOrder(canceled, "id"); err == nil {
		t.Fatalf("expected context error")
	}
}

func TestGetQuoteAndPlaceOrderInputValidation(t *testing.T) {
	b := &Broker{}
	if _, err := b.GetQuote(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "symbol is required") {
		t.Fatalf("expected symbol validation error, got %v", err)
	}
	if _, err := b.PlaceOrder(context.Background(), domain.OrderRequest{Qty: 0}); err == nil || !strings.Contains(err.Error(), "qty") {
		t.Fatalf("expected qty validation error, got %v", err)
	}
	if _, err := b.PlaceOrder(context.Background(), domain.OrderRequest{Qty: 1, Symbol: " "}); err == nil || !strings.Contains(err.Error(), "symbol is required") {
		t.Fatalf("expected symbol validation error, got %v", err)
	}
	if err := b.CancelOrder(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "order id is required") {
		t.Fatalf("expected order id validation error, got %v", err)
	}
}

func TestToFromSDKMappings(t *testing.T) {
	if toSDKSide(domain.SideSell) != sdkalpaca.Sell {
		t.Fatalf("expected sell mapping")
	}
	if toSDKSide(domain.Side("x")) != sdkalpaca.Buy {
		t.Fatalf("expected default buy mapping")
	}
	if toSDKOrderType(domain.OrderTypeLimit) != sdkalpaca.Limit {
		t.Fatalf("expected limit mapping")
	}
	if toSDKOrderType(domain.OrderType("x")) != sdkalpaca.Market {
		t.Fatalf("expected default market mapping")
	}

	if toDomainSide(sdkalpaca.Sell) != domain.SideSell {
		t.Fatalf("expected sell mapping")
	}
	if toDomainSide(sdkalpaca.Buy) != domain.SideBuy {
		t.Fatalf("expected buy mapping")
	}
	if toDomainOrderType(sdkalpaca.Limit) != domain.OrderTypeLimit {
		t.Fatalf("expected limit mapping")
	}
	if toDomainOrderType(sdkalpaca.Market) != domain.OrderTypeMarket {
		t.Fatalf("expected market mapping")
	}
}

func TestToDomainOrderStatusMappings(t *testing.T) {
	tests := map[string]domain.OrderStatus{
		"new":                    domain.OrderStatusNew,
		"accepted":               domain.OrderStatusAccepted,
		"pending_new":            domain.OrderStatusAccepted,
		"partially_filled":       domain.OrderStatusPartially,
		"filled":                 domain.OrderStatusFilled,
		"canceled":               domain.OrderStatusCanceled,
		"cancelled":              domain.OrderStatusCanceled,
		"rejected":               domain.OrderStatusRejected,
		"stopped":                domain.OrderStatusRejected,
		"suspended":              domain.OrderStatusRejected,
		"something_custom_state": domain.OrderStatus("something_custom_state"),
	}
	for in, want := range tests {
		if got := toDomainOrderStatus(in); got != want {
			t.Fatalf("toDomainOrderStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecimalAndTimeHelpers(t *testing.T) {
	d := decimal.NewFromFloat(12.34)
	if got := decimalPtrToFloat(&d); got != 12.34 {
		t.Fatalf("unexpected float conversion: %f", got)
	}
	if got := decimalPtrToFloat(nil); got != 0 {
		t.Fatalf("expected nil to 0, got %f", got)
	}
	if decimalPtrToFloatPtr(nil) != nil {
		t.Fatalf("expected nil pointer conversion")
	}
	if got := decimalPtrToFloatPtr(&d); got == nil || *got != 12.34 {
		t.Fatalf("unexpected pointer conversion: %#v", got)
	}

	fallback := time.Now().UTC()
	if got := nonZeroTime(time.Time{}, fallback); !got.Equal(fallback) {
		t.Fatalf("expected fallback time")
	}
	now := time.Now().UTC()
	if got := nonZeroTime(now, fallback); !got.Equal(now) {
		t.Fatalf("expected original time")
	}
}

func TestToDomainOrderAndTradeUpdate(t *testing.T) {
	qty := decimal.NewFromFloat(2)
	filled := decimal.NewFromFloat(1)
	limit := decimal.NewFromFloat(99.5)
	avgFill := decimal.NewFromFloat(100.25)
	price := decimal.NewFromFloat(101.5)
	now := time.Now().UTC()

	o := sdkalpaca.Order{
		ID:             "ord-1",
		Symbol:         "AAPL",
		Side:           sdkalpaca.Buy,
		Qty:            &qty,
		FilledQty:      filled,
		Type:           sdkalpaca.Limit,
		LimitPrice:     &limit,
		Status:         "new",
		CreatedAt:      now,
		UpdatedAt:      now,
		FilledAvgPrice: &avgFill,
	}
	do := toDomainOrder(o)
	if do.ID != "ord-1" || do.Symbol != "AAPL" || do.Side != domain.SideBuy || do.Type != domain.OrderTypeLimit {
		t.Fatalf("unexpected domain order mapping: %#v", do)
	}
	if do.LimitPrice == nil || *do.LimitPrice != 99.5 {
		t.Fatalf("unexpected limit price mapping: %#v", do.LimitPrice)
	}

	u := sdkalpaca.TradeUpdate{
		At: now,
		Order: sdkalpaca.Order{
			ID:             "ord-1",
			Status:         "filled",
			FilledQty:      filled,
			FilledAvgPrice: &avgFill,
		},
		Price: &price,
	}
	du := toDomainTradeUpdate(u)
	if du.OrderID != "ord-1" || du.Status != domain.OrderStatusFilled || du.FillPrice == nil || *du.FillPrice != 101.5 {
		t.Fatalf("unexpected trade update mapping: %#v", du)
	}

	u.Price = nil
	u.At = time.Time{}
	du2 := toDomainTradeUpdate(u)
	if du2.FillPrice == nil || *du2.FillPrice != 100.25 {
		t.Fatalf("expected fallback filled avg price, got %#v", du2.FillPrice)
	}
	if du2.Time.IsZero() {
		t.Fatalf("expected non-zero fallback time")
	}
}

func TestToDomainStreamQuote(t *testing.T) {
	q := toDomainStreamQuote(stream.Quote{
		Symbol:    "aapl",
		BidPrice:  99.5,
		AskPrice:  100.5,
		Timestamp: time.Now().UTC(),
	})
	if q.Symbol != "AAPL" || q.Bid != 99.5 || q.Ask != 100.5 || q.Last != 100 {
		t.Fatalf("unexpected quote mapping: %#v", q)
	}
}

func TestStreamQuotesEmptySymbols(t *testing.T) {
	b := &Broker{}
	qch, ech, err := b.StreamQuotes(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamQuotes failed: %v", err)
	}
	if _, ok := <-qch; ok {
		t.Fatalf("expected closed quote channel")
	}
	if _, ok := <-ech; ok {
		t.Fatalf("expected closed error channel")
	}
}

func TestDataURLFallbackUsesEnvWithoutPanic(t *testing.T) {
	const envURL = "https://data-env.example"
	old := os.Getenv("APCA_API_DATA_URL")
	_ = os.Setenv("APCA_API_DATA_URL", envURL)
	t.Cleanup(func() {
		_ = os.Setenv("APCA_API_DATA_URL", old)
	})

	b := New(Config{
		BaseURL:   "https://api.alpaca.markets",
		APIKey:    "key",
		APISecret: "secret",
	})
	if b == nil {
		t.Fatalf("expected broker")
	}
}

func TestWatchlistValidationAndNormalizers(t *testing.T) {
	b := &Broker{}
	if _, err := b.GetWatchlistSymbols(" "); err == nil || !strings.Contains(err.Error(), "watchlist name is required") {
		t.Fatalf("expected watchlist name validation, got %v", err)
	}
	if err := b.UpsertWatchlistSymbols(" ", []string{"AAPL"}); err == nil || !strings.Contains(err.Error(), "watchlist name is required") {
		t.Fatalf("expected watchlist name validation, got %v", err)
	}

	got := normalizeSymbols([]string{"aapl", " AAPL ", "msft"})
	if len(got) != 2 || got[0] != "AAPL" || got[1] != "MSFT" {
		t.Fatalf("unexpected normalizeSymbols: %#v", got)
	}

	assets := []sdkalpaca.Asset{
		{Symbol: "aapl"},
		{Symbol: " AAPL "},
		{Symbol: "msft"},
	}
	gotAssets := normalizeSymbolsFromAssets(assets)
	if len(gotAssets) != 2 || gotAssets[0] != "AAPL" || gotAssets[1] != "MSFT" {
		t.Fatalf("unexpected normalizeSymbolsFromAssets: %#v", gotAssets)
	}
}

func TestSDKBackedMethodsWithLocalHTTPServer(t *testing.T) {
	var placedBody map[string]any

	tradeMux := http.NewServeMux()
	tradeMux.HandleFunc("/v2/account", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cash":         "1000",
			"buying_power": "900",
			"equity":       "1100",
		})
	})
	tradeMux.HandleFunc("/v2/positions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"symbol":          "AAPL",
				"qty":             "2",
				"avg_entry_price": "100",
				"current_price":   "101",
			},
		})
	})
	tradeMux.HandleFunc("/v2/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":         "ord-1",
					"symbol":     "AAPL",
					"side":       "buy",
					"qty":        "1",
					"filled_qty": "0",
					"type":       "market",
					"status":     "new",
					"created_at": time.Now().UTC().Format(time.RFC3339Nano),
					"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
				},
			})
		case http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(&placedBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ord-2",
				"symbol":     "AAPL",
				"side":       "buy",
				"qty":        "1",
				"filled_qty": "0",
				"type":       "limit",
				"status":     "new",
				"created_at": time.Now().UTC().Format(time.RFC3339Nano),
				"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	tradeMux.HandleFunc("/v2/orders/ord-2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	dataMux := http.NewServeMux()
	dataMux.HandleFunc("/v2/stocks/quotes/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"quotes": map[string]any{
				"AAPL": map[string]any{
					"t":  time.Now().UTC().Format(time.RFC3339Nano),
					"bp": 99.5,
					"ap": 100.5,
				},
			},
		})
	})

	tradeSrv := httptest.NewServer(tradeMux)
	defer tradeSrv.Close()
	dataSrv := httptest.NewServer(dataMux)
	defer dataSrv.Close()

	b := New(Config{
		BaseURL:     tradeSrv.URL,
		DataBaseURL: dataSrv.URL,
		APIKey:      "key",
		APISecret:   "secret",
		Feed:        "iex",
	})
	ctx := context.Background()

	acct, err := b.GetAccount(ctx)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if acct.Cash != 1000 || acct.BuyingPower != 900 || acct.Equity != 1100 {
		t.Fatalf("unexpected account: %#v", acct)
	}

	pos, err := b.GetPositions(ctx)
	if err != nil {
		t.Fatalf("GetPositions failed: %v", err)
	}
	if len(pos) != 1 || pos[0].Symbol != "AAPL" || pos[0].Qty != 2 || pos[0].AvgCost != 100 || pos[0].LastPrice != 101 {
		t.Fatalf("unexpected positions: %#v", pos)
	}

	orders, err := b.GetOpenOrders(ctx)
	if err != nil {
		t.Fatalf("GetOpenOrders failed: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != "ord-1" {
		t.Fatalf("unexpected open orders: %#v", orders)
	}

	q, err := b.GetQuote(ctx, "aapl")
	if err != nil {
		t.Fatalf("GetQuote failed: %v", err)
	}
	if q.Symbol != "AAPL" || q.Bid != 99.5 || q.Ask != 100.5 || q.Last != 100.0 {
		t.Fatalf("unexpected quote: %#v", q)
	}

	limit := 100.25
	order, err := b.PlaceOrder(ctx, domain.OrderRequest{
		Symbol:        "aapl",
		Side:          domain.SideBuy,
		Qty:           1,
		Type:          domain.OrderTypeLimit,
		LimitPrice:    &limit,
		ClientOrderID: "cid-1",
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if order.ID != "ord-2" || order.Type != domain.OrderTypeLimit {
		t.Fatalf("unexpected placed order: %#v", order)
	}
	if placedBody["symbol"] != "AAPL" || placedBody["client_order_id"] != "cid-1" {
		t.Fatalf("unexpected place order payload: %#v", placedBody)
	}

	if err := b.CancelOrder(ctx, "ord-2"); err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}
}

func TestPlaceOrderMarketIgnoresLimitPrice(t *testing.T) {
	var placedBody map[string]any

	tradeMux := http.NewServeMux()
	tradeMux.HandleFunc("/v2/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&placedBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "ord-mkt-1",
			"symbol":     "AAPL",
			"side":       "buy",
			"qty":        "1",
			"filled_qty": "0",
			"type":       "market",
			"status":     "new",
			"created_at": time.Now().UTC().Format(time.RFC3339Nano),
			"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	})
	tradeSrv := httptest.NewServer(tradeMux)
	defer tradeSrv.Close()

	b := New(Config{
		BaseURL:     tradeSrv.URL,
		DataBaseURL: tradeSrv.URL,
		APIKey:      "key",
		APISecret:   "secret",
		Feed:        "iex",
	})
	ctx := context.Background()
	limit := 0.0
	_, err := b.PlaceOrder(ctx, domain.OrderRequest{
		Symbol:     "AAPL",
		Side:       domain.SideBuy,
		Qty:        1,
		Type:       domain.OrderTypeMarket,
		LimitPrice: &limit,
	})
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}
	if placedBody["type"] != "market" {
		t.Fatalf("expected market order payload, got %#v", placedBody)
	}
	if v, ok := placedBody["limit_price"]; ok && v != nil {
		t.Fatalf("expected nil limit_price for market order payload: %#v", placedBody)
	}
}
