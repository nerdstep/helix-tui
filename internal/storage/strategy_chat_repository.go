package storage

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultRecentStrategyChatThreadLimit  = 20
	defaultRecentStrategyChatMessageLimit = 120
	defaultStrategyChatThreadTitle        = "Default"
)

type StrategyChatThread struct {
	ID            uint
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastMessageAt time.Time
}

type StrategyChatMessage struct {
	ID        uint
	ThreadID  uint
	Role      string
	Content   string
	Model     string
	CreatedAt time.Time
}

type strategyChatThreadRecord struct {
	ID            uint      `gorm:"primaryKey"`
	Title         string    `gorm:"size:160;not null"`
	LastMessageAt time.Time `gorm:"index;not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (strategyChatThreadRecord) TableName() string {
	return "strategy_chat_threads"
}

type strategyChatMessageRecord struct {
	ID        uint      `gorm:"primaryKey"`
	ThreadID  uint      `gorm:"index;not null"`
	Role      string    `gorm:"size:24;index;not null"`
	Content   string    `gorm:"type:text;not null"`
	Model     string    `gorm:"size:128"`
	CreatedAt time.Time `gorm:"index;not null"`
}

func (strategyChatMessageRecord) TableName() string {
	return "strategy_chat_messages"
}

func (r *StrategyRepository) CreateChatThread(title string) (StrategyChatThread, error) {
	if r == nil || r.db == nil {
		return StrategyChatThread{}, fmt.Errorf("strategy repository is not initialized")
	}
	now := time.Now().UTC()
	record := strategyChatThreadRecord{
		Title:         normalizeStrategyChatThreadTitle(title),
		LastMessageAt: now,
	}
	if err := r.db.Create(&record).Error; err != nil {
		return StrategyChatThread{}, fmt.Errorf("insert strategy chat thread: %w", err)
	}
	return fromStrategyChatThreadRecord(record), nil
}

func (r *StrategyRepository) EnsureChatThread(title string) (StrategyChatThread, error) {
	if r == nil || r.db == nil {
		return StrategyChatThread{}, fmt.Errorf("strategy repository is not initialized")
	}
	latest, err := r.GetLatestChatThread()
	if err != nil {
		return StrategyChatThread{}, err
	}
	if latest != nil {
		return *latest, nil
	}
	return r.CreateChatThread(title)
}

func (r *StrategyRepository) GetChatThread(threadID uint) (*StrategyChatThread, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	if threadID == 0 {
		return nil, fmt.Errorf("strategy chat thread id is required")
	}
	var record strategyChatThreadRecord
	result := r.db.Where("id = ?", threadID).Limit(1).Find(&record)
	if result.Error != nil {
		return nil, fmt.Errorf("query strategy chat thread: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	out := fromStrategyChatThreadRecord(record)
	return &out, nil
}

func (r *StrategyRepository) GetLatestChatThread() (*StrategyChatThread, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	var record strategyChatThreadRecord
	result := r.db.
		Order("last_message_at desc, id desc").
		Limit(1).
		Find(&record)
	if result.Error != nil {
		return nil, fmt.Errorf("query latest strategy chat thread: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	out := fromStrategyChatThreadRecord(record)
	return &out, nil
}

func (r *StrategyRepository) ListChatThreads(limit int) ([]StrategyChatThread, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	if limit <= 0 {
		limit = defaultRecentStrategyChatThreadLimit
	}
	var records []strategyChatThreadRecord
	if err := r.db.
		Order("last_message_at desc, id desc").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query strategy chat threads: %w", err)
	}
	out := make([]StrategyChatThread, 0, len(records))
	for _, record := range records {
		out = append(out, fromStrategyChatThreadRecord(record))
	}
	return out, nil
}

func (r *StrategyRepository) AppendChatMessage(threadID uint, role string, content string, model string) (StrategyChatMessage, error) {
	if r == nil || r.db == nil {
		return StrategyChatMessage{}, fmt.Errorf("strategy repository is not initialized")
	}
	if threadID == 0 {
		return StrategyChatMessage{}, fmt.Errorf("strategy chat thread id is required")
	}
	role = normalizeStrategyChatRole(role)
	if role == "" {
		return StrategyChatMessage{}, fmt.Errorf("strategy chat message role is required")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return StrategyChatMessage{}, fmt.Errorf("strategy chat message content is required")
	}
	now := time.Now().UTC()
	record := strategyChatMessageRecord{
		ThreadID:  threadID,
		Role:      role,
		Content:   content,
		Model:     strings.TrimSpace(model),
		CreatedAt: now,
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		var thread strategyChatThreadRecord
		result := tx.Where("id = ?", threadID).Limit(1).Find(&thread)
		if result.Error != nil {
			return fmt.Errorf("query strategy chat thread: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("strategy chat thread %d not found", threadID)
		}
		if err := tx.Create(&record).Error; err != nil {
			return fmt.Errorf("insert strategy chat message: %w", err)
		}
		if err := tx.Model(&strategyChatThreadRecord{}).
			Where("id = ?", threadID).
			Updates(map[string]any{
				"last_message_at": now,
			}).Error; err != nil {
			return fmt.Errorf("update strategy chat thread timestamp: %w", err)
		}
		return nil
	}); err != nil {
		return StrategyChatMessage{}, err
	}
	return fromStrategyChatMessageRecord(record), nil
}

func (r *StrategyRepository) ListChatMessages(threadID uint, limit int) ([]StrategyChatMessage, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("strategy repository is not initialized")
	}
	if threadID == 0 {
		return nil, fmt.Errorf("strategy chat thread id is required")
	}
	if limit <= 0 {
		limit = defaultRecentStrategyChatMessageLimit
	}
	var records []strategyChatMessageRecord
	if err := r.db.
		Where("thread_id = ?", threadID).
		Order("id desc").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("query strategy chat messages: %w", err)
	}
	// Return chronological order for rendering.
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	out := make([]StrategyChatMessage, 0, len(records))
	for _, record := range records {
		out = append(out, fromStrategyChatMessageRecord(record))
	}
	return out, nil
}

func normalizeStrategyChatThreadTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultStrategyChatThreadTitle
	}
	return value
}

func normalizeStrategyChatRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return ""
	}
}

func fromStrategyChatThreadRecord(record strategyChatThreadRecord) StrategyChatThread {
	return StrategyChatThread{
		ID:            record.ID,
		Title:         strings.TrimSpace(record.Title),
		CreatedAt:     record.CreatedAt.UTC(),
		UpdatedAt:     record.UpdatedAt.UTC(),
		LastMessageAt: record.LastMessageAt.UTC(),
	}
}

func fromStrategyChatMessageRecord(record strategyChatMessageRecord) StrategyChatMessage {
	return StrategyChatMessage{
		ID:        record.ID,
		ThreadID:  record.ThreadID,
		Role:      strings.ToLower(strings.TrimSpace(record.Role)),
		Content:   strings.TrimSpace(record.Content),
		Model:     strings.TrimSpace(record.Model),
		CreatedAt: record.CreatedAt.UTC(),
	}
}
