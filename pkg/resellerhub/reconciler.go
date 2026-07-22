package resellerhub

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func (a *App) reconcileOnce(ctx context.Context) {
	if err := a.reconcileQuotaLedgers(ctx); err != nil {
		common.SysError("reseller hub quota reconciliation failed: " + err.Error())
	}
	if err := a.reconcileRetiringTokens(ctx); err != nil {
		common.SysError("reseller hub token retirement reconciliation failed: " + err.Error())
	}
}

func (a *App) reconcileQuotaLedgers(ctx context.Context) error {
	cutoff := time.Now().Add(-a.config.ConsistencyGrace).Unix()
	var ledgers []QuotaLedger
	err := a.db.WithContext(ctx).
		Where("status IN ? AND created_at <= ?", []string{ledgerStatusQuotaApplied, ledgerStatusReconcileRequired}, cutoff).
		Order("id").Limit(100).Find(&ledgers).Error
	if err != nil {
		return err
	}
	for _, ledger := range ledgers {
		var cacheKey, field string
		var status *int
		switch ledger.TargetType {
		case "user_quota":
			cacheKey = "user:" + strconv.Itoa(ledger.NewAPIUserID)
			field = "Quota"
		case "token_quota":
			if ledger.NewAPITokenID == nil {
				continue
			}
			var token model.Token
			if err := a.db.Unscoped().Where("id = ?", *ledger.NewAPITokenID).First(&token).Error; err != nil {
				continue
			}
			cacheKey = "token:" + common.GenerateHMAC(token.Key)
			field = "RemainQuota"
			status = &token.Status
		default:
			continue
		}
		if err := redisHealthy(ctx); err != nil {
			return err
		}
		if _, err := a.applyRedisQuotaEvent(ctx, cacheKey, field, ledger.EventID, ledger.QuotaDelta, status); err != nil {
			_ = a.db.Model(&QuotaLedger{}).Where("id = ?", ledger.ID).Updates(map[string]any{"status": ledgerStatusReconcileRequired, "error_message": err.Error()}).Error
			continue
		}
		now := time.Now().Unix()
		if err := a.db.Model(&QuotaLedger{}).Where("id = ?", ledger.ID).Updates(map[string]any{"status": ledgerStatusApplied, "error_message": "", "applied_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (a *App) reconcileRetiringTokens(ctx context.Context) error {
	var mappings []CustomerToken
	if err := a.db.WithContext(ctx).Where("status = ?", "retiring").Order("id").Limit(100).Find(&mappings).Error; err != nil {
		return err
	}
	for _, mapping := range mappings {
		var audit AuditLog
		objectID := strconv.Itoa(mapping.NewAPITokenID)
		if err := a.db.Where("reseller_id = ? AND action = ? AND object_type = ? AND object_id = ?", mapping.ResellerID, "token.retire", "token", objectID).Order("id DESC").First(&audit).Error; err != nil {
			continue
		}
		if time.Since(time.Unix(audit.CreatedAt, 0)) < a.config.RetirementObservation {
			continue
		}
		var tasks []model.Task
		if err := a.db.WithContext(ctx).
			Where("user_id = ? AND status NOT IN ?", mapping.QuotaCarrierUserID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).
			Limit(1000).Find(&tasks).Error; err != nil {
			return err
		}
		active := false
		for _, task := range tasks {
			if task.PrivateData.TokenId == mapping.NewAPITokenID {
				active = true
				break
			}
		}
		if active {
			continue
		}
		now := time.Now().Unix()
		err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&CustomerToken{}).Where("id = ? AND status = ?", mapping.ID, "retiring").Updates(map[string]any{"status": "retired", "ended_at": now})
			if result.Error != nil || result.RowsAffected != 1 {
				return result.Error
			}
			return tx.Model(&Customer{}).Where("id = ? AND active_token_mapping_id = ?", mapping.CustomerID, mapping.ID).Update("active_token_mapping_id", nil).Error
		})
		if err != nil {
			return fmt.Errorf("retire token mapping %d: %w", mapping.ID, err)
		}
	}
	return nil
}
