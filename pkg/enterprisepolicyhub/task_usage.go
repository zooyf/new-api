package enterprisepolicyhub

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const taskSettlementGracePeriod = 30 * time.Second

func (a *App) settlePendingTaskBudgetTransactions(taskID string) (int, error) {
	if taskID == "" {
		return 0, nil
	}
	var transactions []BudgetTransaction
	if err := a.db.Where("task_id = ? AND pending = ?", taskID, true).Find(&transactions).Error; err != nil {
		return 0, err
	}
	if len(transactions) == 0 {
		return 0, nil
	}

	pendingByAccount := make(map[int]int)
	keyID := 0
	for _, transaction := range transactions {
		pendingByAccount[transaction.BudgetAccountID] += transaction.Quota
		if keyID == 0 {
			keyID = transaction.EnterpriseKeyID
		}
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&BudgetTransaction{}).
			Where("task_id = ? AND pending = ?", taskID, true).
			Update("pending", false).Error; err != nil {
			return err
		}
		for accountID, pendingQuota := range pendingByAccount {
			if err := tx.Model(&BudgetAccount{}).Where("id = ?", accountID).Update(
				"pending_quota",
				gorm.Expr("CASE WHEN pending_quota >= ? THEN pending_quota - ? ELSE 0 END", pendingQuota, pendingQuota),
			).Error; err != nil {
				return err
			}
		}
		return tx.Model(&OrganizationUsageLedger{}).
			Where("task_id = ? AND usage_state = ?", taskID, UsageStatePending).
			Update("usage_state", UsageStateSettled).Error
	}); err != nil {
		return 0, err
	}

	var key EnterpriseKey
	if keyID > 0 {
		_ = a.db.First(&key, keyID).Error
	}
	disabled := 0
	for accountID := range pendingByAccount {
		var account BudgetAccount
		if err := a.db.First(&account, accountID).Error; err != nil {
			return disabled, err
		}
		if budgetShouldBlock(account, time.Now().Unix()) {
			count, err := a.ensureBudgetBlocks(account, key)
			if err != nil {
				return disabled, err
			}
			disabled += count
		} else if _, err := a.releaseBudgetBlocks(account.ID); err != nil {
			return disabled, err
		}
	}
	return disabled, nil
}

func (a *App) reconcileCompletedTaskUsage(now int64) (int, error) {
	type pendingTask struct {
		TaskID        string
		NewAPILogID   int
		NewAPITokenID int
	}
	var pending []pendingTask
	if err := a.db.Model(&OrganizationUsageLedger{}).
		Select("task_id, MIN(new_api_log_id) AS new_api_log_id, MIN(new_api_token_id) AS new_api_token_id").
		Where("usage_state = ? AND task_id <> ''", UsageStatePending).
		Group("task_id").Limit(1000).Scan(&pending).Error; err != nil {
		return 0, err
	}
	disabled := 0
	for _, item := range pending {
		var task model.Task
		if err := a.newAPIDB.Where("task_id = ?", item.TaskID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return disabled, err
		}
		if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
			continue
		}
		finishedAt := task.FinishTime
		if finishedAt <= 0 {
			finishedAt = task.UpdatedAt
		}
		if finishedAt <= 0 || now-finishedAt < int64(taskSettlementGracePeriod/time.Second) {
			continue
		}

		var settledLedgerCount int64
		if err := a.db.Model(&OrganizationUsageLedger{}).
			Where("task_id = ? AND usage_state = ? AND new_api_log_id > ?", item.TaskID, UsageStateSettled, item.NewAPILogID).
			Count(&settledLedgerCount).Error; err != nil {
			return disabled, err
		}
		if settledLedgerCount == 0 {
			var pendingSettlementLogCount int64
			if err := a.newAPILog.Model(&model.Log{}).
				Where("id > ? AND token_id = ? AND other LIKE ?", item.NewAPILogID, item.NewAPITokenID, "%"+item.TaskID+"%").
				Count(&pendingSettlementLogCount).Error; err != nil {
				return disabled, err
			}
			if pendingSettlementLogCount > 0 {
				continue
			}
		}
		count, err := a.settlePendingTaskBudgetTransactions(item.TaskID)
		if err != nil {
			return disabled, err
		}
		disabled += count
	}
	return disabled, nil
}
