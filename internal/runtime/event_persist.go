package runtime

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"helix-tui/internal/domain"
)

const (
	tradeEventPersistBatchSize = 32
	tradeEventPersistQueueSize = 512
	tradeEventPersistFlush     = 500 * time.Millisecond
	tradeEventPersistReport    = 30 * time.Second
)

type tradeEventAppender interface {
	AppendMany(events []domain.Event) error
}

type tradeEventPersistor struct {
	appender tradeEventAppender
	ch       chan domain.Event
	stop     chan struct{}
	done     chan struct{}
	report   func(domain.Event)

	mu        sync.Mutex
	closeOnce sync.Once
	stats     tradeEventPersistStats
}

type tradeEventPersistStats struct {
	FlushOK      int64
	FlushFailed  int64
	EventsOK     int64
	EventsFailed int64
	Dropped      int64
	LastError    string
}

func newTradeEventPersistor(appender tradeEventAppender, report func(domain.Event)) *tradeEventPersistor {
	p := &tradeEventPersistor{
		appender: appender,
		ch:       make(chan domain.Event, tradeEventPersistQueueSize),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		report:   report,
	}
	go p.run()
	return p
}

func (p *tradeEventPersistor) HandleEvent(event domain.Event) {
	if p == nil || p.appender == nil || !isPersistedTradeEventType(event.Type) {
		return
	}
	select {
	case <-p.stop:
		return
	default:
	}
	select {
	case p.ch <- event:
	default:
		p.mu.Lock()
		p.stats.Dropped++
		p.mu.Unlock()
	}
}

func (p *tradeEventPersistor) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
}

func (p *tradeEventPersistor) run() {
	defer close(p.done)

	batch := make([]domain.Event, 0, tradeEventPersistBatchSize)
	ticker := time.NewTicker(tradeEventPersistFlush)
	reportTicker := time.NewTicker(tradeEventPersistReport)
	defer ticker.Stop()
	defer reportTicker.Stop()

	lastReported := tradeEventPersistStats{}
	lastReportedQueue := -1

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := p.appender.AppendMany(batch); err != nil {
			p.mu.Lock()
			p.stats.FlushFailed++
			p.stats.EventsFailed += int64(len(batch))
			p.stats.LastError = err.Error()
			p.mu.Unlock()
			p.emitPersistError(err)
			log.Printf("trade event persist failed: %v", err)
		} else {
			p.mu.Lock()
			p.stats.FlushOK++
			p.stats.EventsOK += int64(len(batch))
			p.mu.Unlock()
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-p.stop:
			drain := true
			for drain {
				select {
				case event := <-p.ch:
					batch = append(batch, event)
					if len(batch) >= tradeEventPersistBatchSize {
						flush()
					}
				default:
					drain = false
				}
			}
			flush()
			p.emitStats(true, &lastReported, &lastReportedQueue)
			return
		case event := <-p.ch:
			batch = append(batch, event)
			if len(batch) >= tradeEventPersistBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-reportTicker.C:
			p.emitStats(false, &lastReported, &lastReportedQueue)
		}
	}
}

func (p *tradeEventPersistor) emitPersistError(err error) {
	if p == nil || p.report == nil || err == nil {
		return
	}
	p.report(domain.Event{
		Type:    "event_persist_error",
		Details: strings.TrimSpace(err.Error()),
		Time:    time.Now().UTC(),
	})
}

func (p *tradeEventPersistor) emitStats(force bool, lastReported *tradeEventPersistStats, lastReportedQueue *int) {
	if p == nil || p.report == nil {
		return
	}
	p.mu.Lock()
	stats := p.stats
	p.mu.Unlock()
	queueDepth := len(p.ch)
	if !force && lastReported != nil && lastReportedQueue != nil && *lastReported == stats && *lastReportedQueue == queueDepth {
		return
	}
	p.report(domain.Event{
		Type: "event_persist_stats",
		Time: time.Now().UTC(),
		Details: fmt.Sprintf(
			"queue=%d flush_ok=%d flush_failed=%d events_ok=%d events_failed=%d dropped=%d",
			queueDepth,
			stats.FlushOK,
			stats.FlushFailed,
			stats.EventsOK,
			stats.EventsFailed,
			stats.Dropped,
		),
	})
	if lastReported != nil {
		*lastReported = stats
	}
	if lastReportedQueue != nil {
		*lastReportedQueue = queueDepth
	}
}

func isPersistedTradeEventType(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "order_placed", "order_canceled", "trade_update", "trade_update_unknown_order", "agent_intent_executed", "agent_intent_rejected":
		return true
	default:
		return false
	}
}
