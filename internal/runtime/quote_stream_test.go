package runtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
)

func TestStartQuoteStreamingUpdatesEngineCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := engine.New(&quoteStreamTestBroker{}, engine.NewRiskGate(engine.Policy{AllowMarketOrders: true}))
	streamer := &fakeQuoteStreamer{}
	system := &app.System{
		Engine:        eng,
		QuoteStreamer: streamer,
		Watchlist:     []string{"aapl"},
	}

	update := startQuoteStreaming(ctx, system)
	if got := streamer.callCount(); got != 1 {
		t.Fatalf("expected one initial stream call, got %d", got)
	}
	if got := streamer.callSymbols(0); len(got) != 1 || got[0] != "AAPL" {
		t.Fatalf("unexpected initial stream symbols: %#v", got)
	}

	streamer.sendQuote(0, domain.Quote{Symbol: "AAPL", Last: 123.45, Time: time.Now().UTC()})
	waitForRuntime(t, time.Second, func() bool {
		q, err := eng.GetQuote(context.Background(), "AAPL")
		return err == nil && q.Last == 123.45
	})

	update([]string{"msft"})
	if got := streamer.callCount(); got != 2 {
		t.Fatalf("expected second stream call after update, got %d", got)
	}
	if got := streamer.callSymbols(1); len(got) != 1 || got[0] != "MSFT" {
		t.Fatalf("unexpected updated stream symbols: %#v", got)
	}
	select {
	case <-streamer.callCtxDone(0):
	case <-time.After(time.Second):
		t.Fatalf("expected first stream context to be canceled")
	}
}

func TestStartQuoteStreamingNoopWithoutStreamer(t *testing.T) {
	update := startQuoteStreaming(context.Background(), &app.System{})
	update([]string{"AAPL"})
}

type quoteStreamTestBroker struct{}

func (quoteStreamTestBroker) GetAccount(context.Context) (domain.Account, error) {
	return domain.Account{}, nil
}

func (quoteStreamTestBroker) GetPositions(context.Context) ([]domain.Position, error) {
	return nil, nil
}

func (quoteStreamTestBroker) GetOpenOrders(context.Context) ([]domain.Order, error) {
	return nil, nil
}

func (quoteStreamTestBroker) GetQuote(context.Context, string) (domain.Quote, error) {
	return domain.Quote{}, fmt.Errorf("no broker quote")
}

func (quoteStreamTestBroker) PlaceOrder(context.Context, domain.OrderRequest) (domain.Order, error) {
	return domain.Order{}, nil
}

func (quoteStreamTestBroker) CancelOrder(context.Context, string) error {
	return nil
}

func (quoteStreamTestBroker) StreamTradeUpdates(context.Context) (<-chan domain.TradeUpdate, error) {
	ch := make(chan domain.TradeUpdate)
	close(ch)
	return ch, nil
}

type fakeQuoteStreamer struct {
	mu    sync.Mutex
	calls []fakeQuoteStreamCall
}

type fakeQuoteStreamCall struct {
	symbols []string
	ctx     context.Context
	quotes  chan domain.Quote
	errs    chan error
}

func (f *fakeQuoteStreamer) StreamQuotes(ctx context.Context, symbols []string) (<-chan domain.Quote, <-chan error, error) {
	call := fakeQuoteStreamCall{
		symbols: append([]string{}, symbols...),
		ctx:     ctx,
		quotes:  make(chan domain.Quote, 8),
		errs:    make(chan error, 8),
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()

	go func(quotes chan domain.Quote, errs chan error) {
		<-ctx.Done()
		close(quotes)
		close(errs)
	}(call.quotes, call.errs)

	return call.quotes, call.errs, nil
}

func (f *fakeQuoteStreamer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeQuoteStreamer) callSymbols(i int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.calls) {
		return nil
	}
	return append([]string{}, f.calls[i].symbols...)
}

func (f *fakeQuoteStreamer) callCtxDone(i int) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.calls) {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return f.calls[i].ctx.Done()
}

func (f *fakeQuoteStreamer) sendQuote(i int, quote domain.Quote) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.calls) {
		return
	}
	f.calls[i].quotes <- quote
}

func waitForRuntime(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
