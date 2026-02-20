package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"helix-tui/internal/app"
	"helix-tui/internal/domain"
	"helix-tui/internal/engine"
	"helix-tui/internal/symbols"
)

type quoteStreamController struct {
	parentCtx context.Context
	engine    *engine.Engine
	streamer  domain.QuoteStreamer

	mu          sync.Mutex
	cancel      context.CancelFunc
	lastSymbols []string
}

func startQuoteStreaming(ctx context.Context, system *app.System) func([]string) {
	if system == nil || system.Engine == nil || system.QuoteStreamer == nil {
		return func([]string) {}
	}
	controller := &quoteStreamController{
		parentCtx: ctx,
		engine:    system.Engine,
		streamer:  system.QuoteStreamer,
	}
	controller.Update(system.Watchlist)
	return controller.Update
}

func (c *quoteStreamController) Update(next []string) {
	syms := symbols.Normalize(next)

	c.mu.Lock()
	if sameSymbols(c.lastSymbols, syms) {
		c.mu.Unlock()
		return
	}
	c.lastSymbols = append([]string{}, syms...)
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if len(syms) == 0 {
		c.mu.Unlock()
		c.engine.AddEvent("quote_stream_stop", "watchlist empty")
		return
	}
	streamCtx, cancel := context.WithCancel(c.parentCtx)
	c.cancel = cancel
	c.mu.Unlock()

	quotes, errs, err := c.streamer.StreamQuotes(streamCtx, syms)
	if err != nil {
		cancel()
		c.engine.AddEvent("quote_stream_error", fmt.Sprintf("start symbols=%s: %v", strings.Join(syms, ","), err))
		return
	}
	c.engine.AddEvent("quote_stream_start", fmt.Sprintf("symbols=%s", strings.Join(syms, ",")))
	go c.consume(streamCtx, quotes, errs)
}

func (c *quoteStreamController) consume(ctx context.Context, quotes <-chan domain.Quote, errs <-chan error) {
	for quotes != nil || errs != nil {
		select {
		case <-ctx.Done():
			return
		case q, ok := <-quotes:
			if !ok {
				quotes = nil
				continue
			}
			c.engine.UpsertQuote(q)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				c.engine.AddEvent("quote_stream_error", err.Error())
			}
		}
	}
}

func sameSymbols(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
