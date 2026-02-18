package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"helix-tui/internal/domain"
)

const (
	PaperAPIBase = "https://paper-api.alpaca.markets"
	LiveAPIBase  = "https://api.alpaca.markets"
)

type Broker struct {
	baseURL string
	apiKey  string
	secret  string
	client  *http.Client
}

func New(baseURL, apiKey, secret string) *Broker {
	return &Broker{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		secret:  secret,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func NewPaper(apiKey, secret string) *Broker {
	return New(PaperAPIBase, apiKey, secret)
}

func (b *Broker) GetAccount(ctx context.Context) (domain.Account, error) {
	var payload struct {
		Cash        string `json:"cash"`
		BuyingPower string `json:"buying_power"`
		Equity      string `json:"equity"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/v2/account", nil, &payload); err != nil {
		return domain.Account{}, err
	}
	cash, _ := strconv.ParseFloat(payload.Cash, 64)
	buyingPower, _ := strconv.ParseFloat(payload.BuyingPower, 64)
	equity, _ := strconv.ParseFloat(payload.Equity, 64)
	return domain.Account{
		Cash:        cash,
		BuyingPower: buyingPower,
		Equity:      equity,
	}, nil
}

func (b *Broker) GetPositions(ctx context.Context) ([]domain.Position, error) {
	var payload []struct {
		Symbol        string `json:"symbol"`
		Qty           string `json:"qty"`
		AvgEntryPrice string `json:"avg_entry_price"`
		CurrentPrice  string `json:"current_price"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/v2/positions", nil, &payload); err != nil {
		return nil, err
	}
	out := make([]domain.Position, 0, len(payload))
	for _, p := range payload {
		qty, _ := strconv.ParseFloat(p.Qty, 64)
		avgCost, _ := strconv.ParseFloat(p.AvgEntryPrice, 64)
		lastPrice, _ := strconv.ParseFloat(p.CurrentPrice, 64)
		out = append(out, domain.Position{
			Symbol:    p.Symbol,
			Qty:       qty,
			AvgCost:   avgCost,
			LastPrice: lastPrice,
		})
	}
	return out, nil
}

func (b *Broker) GetOpenOrders(ctx context.Context) ([]domain.Order, error) {
	var payload []struct {
		ID         string `json:"id"`
		Symbol     string `json:"symbol"`
		Side       string `json:"side"`
		Qty        string `json:"qty"`
		FilledQty  string `json:"filled_qty"`
		Type       string `json:"type"`
		LimitPrice string `json:"limit_price"`
		Status     string `json:"status"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := b.doJSON(ctx, http.MethodGet, "/v2/orders?status=open", nil, &payload); err != nil {
		return nil, err
	}
	out := make([]domain.Order, 0, len(payload))
	for _, o := range payload {
		qty, _ := strconv.ParseFloat(o.Qty, 64)
		filledQty, _ := strconv.ParseFloat(o.FilledQty, 64)
		var limit *float64
		if o.LimitPrice != "" {
			p, _ := strconv.ParseFloat(o.LimitPrice, 64)
			limit = &p
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, o.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339Nano, o.UpdatedAt)
		out = append(out, domain.Order{
			ID:         o.ID,
			Symbol:     o.Symbol,
			Side:       domain.Side(o.Side),
			Qty:        qty,
			FilledQty:  filledQty,
			Type:       domain.OrderType(o.Type),
			LimitPrice: limit,
			Status:     domain.OrderStatus(o.Status),
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		})
	}
	return out, nil
}

func (b *Broker) GetQuote(_ context.Context, _ string) (domain.Quote, error) {
	return domain.Quote{}, fmt.Errorf("alpaca market data quote path is not wired in this scaffold")
}

func (b *Broker) PlaceOrder(ctx context.Context, req domain.OrderRequest) (domain.Order, error) {
	body := map[string]any{
		"symbol":        strings.ToUpper(strings.TrimSpace(req.Symbol)),
		"side":          string(req.Side),
		"type":          string(req.Type),
		"time_in_force": "day",
		"qty":           fmt.Sprintf("%.6f", req.Qty),
	}
	if req.ClientOrderID != "" {
		body["client_order_id"] = req.ClientOrderID
	}
	if req.LimitPrice != nil {
		body["limit_price"] = fmt.Sprintf("%.6f", *req.LimitPrice)
	}

	var payload struct {
		ID         string `json:"id"`
		Symbol     string `json:"symbol"`
		Side       string `json:"side"`
		Qty        string `json:"qty"`
		FilledQty  string `json:"filled_qty"`
		Type       string `json:"type"`
		LimitPrice string `json:"limit_price"`
		Status     string `json:"status"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := b.doJSON(ctx, http.MethodPost, "/v2/orders", body, &payload); err != nil {
		return domain.Order{}, err
	}

	qty, _ := strconv.ParseFloat(payload.Qty, 64)
	filledQty, _ := strconv.ParseFloat(payload.FilledQty, 64)
	var limit *float64
	if payload.LimitPrice != "" {
		p, _ := strconv.ParseFloat(payload.LimitPrice, 64)
		limit = &p
	}
	createdAt, _ := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, payload.UpdatedAt)

	return domain.Order{
		ID:         payload.ID,
		Symbol:     payload.Symbol,
		Side:       domain.Side(payload.Side),
		Qty:        qty,
		FilledQty:  filledQty,
		Type:       domain.OrderType(payload.Type),
		LimitPrice: limit,
		Status:     domain.OrderStatus(payload.Status),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

func (b *Broker) CancelOrder(ctx context.Context, orderID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, b.baseURL+"/v2/orders/"+orderID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("APCA-API-KEY-ID", b.apiKey)
	req.Header.Set("APCA-API-SECRET-KEY", b.secret)

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("alpaca cancel failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (b *Broker) StreamTradeUpdates(_ context.Context) (<-chan domain.TradeUpdate, error) {
	return nil, fmt.Errorf("alpaca websocket streaming is not wired in this scaffold")
}

func (b *Broker) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(in); err != nil {
			return err
		}
		body = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, b.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("APCA-API-KEY-ID", b.apiKey)
	req.Header.Set("APCA-API-SECRET-KEY", b.secret)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("alpaca request failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
