package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	UpstreamEventStatusPending    = "pending"
	UpstreamEventStatusDelivering = "delivering"
	UpstreamEventStatusDelivered  = "delivered"
	UpstreamEventStatusRetrying   = "retrying"
	UpstreamEventStatusDead       = "dead"
)

type UpstreamEventOutbox struct {
	ID                int    `json:"id" gorm:"primaryKey"`
	EventID           string `json:"event_id" gorm:"type:varchar(64);uniqueIndex"`
	EventType         string `json:"event_type" gorm:"type:varchar(64);index"`
	Status            string `json:"status" gorm:"type:varchar(24);index"`
	Priority          int    `json:"priority" gorm:"index"`
	SourceSystem      string `json:"source_system" gorm:"type:varchar(128);index"`
	RequestID         string `json:"request_id" gorm:"type:varchar(64);index"`
	UpstreamRequestID string `json:"upstream_request_id" gorm:"type:varchar(128);index"`
	TaskID            string `json:"task_id" gorm:"type:varchar(191);index"`
	UpstreamTaskID    string `json:"upstream_task_id" gorm:"type:varchar(191);index"`
	Payload           string `json:"payload" gorm:"type:text"`
	DeliveryAttempts  int    `json:"delivery_attempts"`
	NextRetryAt       int64  `json:"next_retry_at" gorm:"index"`
	DeliveredAt       int64  `json:"delivered_at"`
	LastDeliveryError string `json:"last_delivery_error" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at" gorm:"index"`
	UpdatedAt         int64  `json:"updated_at"`
}

func (e *UpstreamEventOutbox) BeforeCreate(_ *gorm.DB) error {
	now := time.Now().Unix()
	if e.CreatedAt == 0 {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	if e.Status == "" {
		e.Status = UpstreamEventStatusPending
	}
	return nil
}

func (e *UpstreamEventOutbox) BeforeUpdate(_ *gorm.DB) error {
	e.UpdatedAt = time.Now().Unix()
	return nil
}

func CreateUpstreamEventOutbox(event *UpstreamEventOutbox) error {
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(event).Error
}

func ListUpstreamEventOutbox(status string, eventType string, offset int, limit int) ([]UpstreamEventOutbox, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := DB.Model(&UpstreamEventOutbox{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if eventType != "" {
		query = query.Where("event_type = ?", eventType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var events []UpstreamEventOutbox
	err := query.Order("id desc").Offset(offset).Limit(limit).Find(&events).Error
	return events, total, err
}

func GetUpstreamEventOutbox(id int) (*UpstreamEventOutbox, error) {
	var event UpstreamEventOutbox
	err := DB.First(&event, "id = ?", id).Error
	return &event, err
}

func LeaseUpstreamEventOutbox(limit int, now int64) ([]UpstreamEventOutbox, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var candidates []UpstreamEventOutbox
	err := DB.Where(
		"status = ? OR (status = ? AND next_retry_at <= ?)",
		UpstreamEventStatusPending,
		UpstreamEventStatusRetrying,
		now,
	).Order("priority asc, id asc").Limit(limit).Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	leased := make([]UpstreamEventOutbox, 0, len(candidates))
	for _, event := range candidates {
		res := DB.Model(&UpstreamEventOutbox{}).
			Where("id = ? AND (status = ? OR status = ?)", event.ID, UpstreamEventStatusPending, UpstreamEventStatusRetrying).
			Updates(map[string]interface{}{
				"status":              UpstreamEventStatusDelivering,
				"delivery_attempts":   gorm.Expr("delivery_attempts + ?", 1),
				"last_delivery_error": "",
				"updated_at":          now,
			})
		if res.Error != nil {
			return leased, res.Error
		}
		if res.RowsAffected == 1 {
			event.Status = UpstreamEventStatusDelivering
			event.DeliveryAttempts++
			leased = append(leased, event)
		}
	}
	return leased, nil
}

func MarkUpstreamEventOutboxDelivered(id int, deliveredAt int64) error {
	return DB.Model(&UpstreamEventOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":              UpstreamEventStatusDelivered,
		"delivered_at":        deliveredAt,
		"last_delivery_error": "",
		"updated_at":          deliveredAt,
	}).Error
}

func MarkUpstreamEventOutboxRetrying(id int, nextRetryAt int64, lastErr string) error {
	return DB.Model(&UpstreamEventOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":              UpstreamEventStatusRetrying,
		"next_retry_at":       nextRetryAt,
		"last_delivery_error": lastErr,
		"updated_at":          time.Now().Unix(),
	}).Error
}

func MarkUpstreamEventOutboxDead(id int, lastErr string) error {
	now := time.Now().Unix()
	return DB.Model(&UpstreamEventOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":              UpstreamEventStatusDead,
		"last_delivery_error": lastErr,
		"updated_at":          now,
	}).Error
}

func RetryUpstreamEventOutbox(id int) error {
	now := time.Now().Unix()
	return DB.Model(&UpstreamEventOutbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":              UpstreamEventStatusRetrying,
		"next_retry_at":       now,
		"last_delivery_error": "",
		"updated_at":          now,
	}).Error
}

func GetUpstreamEventOutboxStats() (map[string]int64, error) {
	rows, err := DB.Model(&UpstreamEventOutbox{}).Select("status, count(*) as count").Group("status").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, rows.Err()
}
