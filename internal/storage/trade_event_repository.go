package storage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"helix-tui/internal/domain"
)

const defaultRecentEventLimit = 200

type tradeEventRecord struct {
	ID              uint      `gorm:"primaryKey"`
	Time            time.Time `gorm:"index;not null"`
	EventType       string    `gorm:"size:128;index;not null"`
	Symbol          string    `gorm:"size:32;index"`
	OrderID         string    `gorm:"size:128;index"`
	Side            string    `gorm:"size:16;index"`
	Qty             float64   `gorm:"not null;default:0"`
	OrderType       string    `gorm:"size:16"`
	OrderStatus     string    `gorm:"size:32;index"`
	Confidence      float64   `gorm:"not null;default:0"`
	ExpectedGainPct float64   `gorm:"not null;default:0"`
	Rationale       string    `gorm:"type:text"`
	RejectionReason string    `gorm:"type:text"`
	Details         string    `gorm:"type:text;not null"`
	CreatedAt       time.Time
}

func (tradeEventRecord) TableName() string {
	return "trade_events"
}

type TradeEventRepository struct {
	db *gorm.DB
}

var (
	orderPlacedPattern    = regexp.MustCompile(`^(buy|sell)\s+([A-Z][A-Z0-9.\-]*)\s+([0-9]+(?:\.[0-9]+)?)\s+\(([^)]+)\)$`)
	tradeUpdatePattern    = regexp.MustCompile(`^([^\s]+)\s+status=([a-z_]+)\s+filled=([0-9]+(?:\.[0-9]+)?)$`)
	executedIntentPattern = regexp.MustCompile(`^(buy|sell)\s+([A-Z][A-Z0-9.\-]*)\s+qty=([0-9]+(?:\.[0-9]+)?)\s+type=([a-z_]+)\s+conf=([-0-9.]+)\s+gain=([-0-9.]+)%\s+rationale=(.*)$`)
	rejectedIntentPattern = regexp.MustCompile(`^(buy|sell)\s+([A-Z][A-Z0-9.\-]*)\s+qty=([0-9]+(?:\.[0-9]+)?)\s+type=([a-z_]+)\s+conf=([-0-9.]+)\s+gain=([-0-9.]+)%\s+rejection=(.*)$`)
)

func (r *TradeEventRepository) Append(event domain.Event) error {
	return r.AppendMany([]domain.Event{event})
}

func (r *TradeEventRepository) AppendMany(events []domain.Event) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("trade event repository is not initialized")
	}
	if len(events) == 0 {
		return nil
	}
	records := make([]tradeEventRecord, 0, len(events))
	for _, event := range events {
		record := toTradeEventRecord(event)
		if record.EventType == "" {
			return fmt.Errorf("trade event type is required")
		}
		records = append(records, record)
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&records).Error
	}); err != nil {
		return fmt.Errorf("insert trade events: %w", err)
	}
	return nil
}

func (r *TradeEventRepository) ListRecent(limit int) ([]domain.Event, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("trade event repository is not initialized")
	}
	if limit <= 0 {
		limit = defaultRecentEventLimit
	}

	var records []tradeEventRecord
	if err := r.db.
		Where("event_type <> ''").
		Order("time desc, id desc").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query trade events: %w", err)
	}

	// Return oldest->newest ordering.
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}

	out := make([]domain.Event, 0, len(records))
	for _, rec := range records {
		out = append(out, toDomainEvent(rec))
	}
	return out, nil
}

func toTradeEventRecord(event domain.Event) tradeEventRecord {
	record := tradeEventRecord{
		Time:      event.Time.UTC(),
		EventType: strings.ToLower(strings.TrimSpace(event.Type)),
		Details:   strings.TrimSpace(event.Details),
	}
	if record.Time.IsZero() {
		record.Time = time.Now().UTC()
	}

	switch record.EventType {
	case "order_placed":
		if parts := orderPlacedPattern.FindStringSubmatch(record.Details); len(parts) == 5 {
			record.Side = parts[1]
			record.Symbol = parts[2]
			record.Qty = parseFloat(parts[3])
			record.OrderID = parts[4]
		}
	case "order_canceled", "trade_update_unknown_order":
		record.OrderID = strings.TrimSpace(record.Details)
	case "trade_update":
		if parts := tradeUpdatePattern.FindStringSubmatch(record.Details); len(parts) == 4 {
			record.OrderID = parts[1]
			record.OrderStatus = parts[2]
			record.Qty = parseFloat(parts[3])
		}
	case "agent_intent_executed":
		if parts := executedIntentPattern.FindStringSubmatch(record.Details); len(parts) == 8 {
			record.Side = parts[1]
			record.Symbol = parts[2]
			record.Qty = parseFloat(parts[3])
			record.OrderType = parts[4]
			record.Confidence = parseFloat(parts[5])
			record.ExpectedGainPct = parseFloat(parts[6])
			record.Rationale = strings.TrimSpace(parts[7])
		}
	case "agent_intent_rejected":
		if parts := rejectedIntentPattern.FindStringSubmatch(record.Details); len(parts) == 8 {
			record.Side = parts[1]
			record.Symbol = parts[2]
			record.Qty = parseFloat(parts[3])
			record.OrderType = parts[4]
			record.Confidence = parseFloat(parts[5])
			record.ExpectedGainPct = parseFloat(parts[6])
			record.RejectionReason = strings.TrimSpace(parts[7])
		}
	default:
		// keep details only
	}
	return record
}

func toDomainEvent(record tradeEventRecord) domain.Event {
	details := strings.TrimSpace(record.Details)
	switch record.EventType {
	case "order_placed":
		if record.Side != "" && record.Symbol != "" && record.OrderID != "" {
			details = fmt.Sprintf("%s %s %.2f (%s)", record.Side, record.Symbol, record.Qty, record.OrderID)
		}
	case "order_canceled", "trade_update_unknown_order":
		if record.OrderID != "" {
			details = record.OrderID
		}
	case "trade_update":
		if record.OrderID != "" && record.OrderStatus != "" {
			details = fmt.Sprintf("%s status=%s filled=%.2f", record.OrderID, record.OrderStatus, record.Qty)
		}
	case "agent_intent_executed":
		if record.Side != "" && record.Symbol != "" {
			details = fmt.Sprintf(
				"%s %s qty=%.2f type=%s conf=%.2f gain=%.2f%% rationale=%s",
				record.Side,
				record.Symbol,
				record.Qty,
				record.OrderType,
				record.Confidence,
				record.ExpectedGainPct,
				strings.TrimSpace(record.Rationale),
			)
		}
	case "agent_intent_rejected":
		if record.Side != "" && record.Symbol != "" {
			details = fmt.Sprintf(
				"%s %s qty=%.2f type=%s conf=%.2f gain=%.2f%%",
				record.Side,
				record.Symbol,
				record.Qty,
				record.OrderType,
				record.Confidence,
				record.ExpectedGainPct,
			)
		}
	}
	return domain.Event{
		Time:            record.Time.UTC(),
		Type:            record.EventType,
		Details:         details,
		RejectionReason: strings.TrimSpace(record.RejectionReason),
	}
}

func parseFloat(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return value
}
