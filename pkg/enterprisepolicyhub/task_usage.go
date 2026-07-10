package enterprisepolicyhub

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const taskSettlementGracePeriod = 30 * time.Second

func (a *App) settlePendingTaskBudgetTransactions(taskID string, settledAt int64) (int, error) {
	if taskID == "" {
		return 0, nil
	}
	if settledAt <= 0 {
		settledAt = time.Now().Unix()
	}
	var ledgers []OrganizationUsageLedger
	if err := a.db.Where("task_id = ? AND usage_state IN ?", taskID, []string{UsageStatePending, UsageStateSettling}).Find(&ledgers).Error; err != nil {
		return 0, err
	}
	if len(ledgers) == 0 {
		return 0, nil
	}
	if err := a.ensurePolicyBudgetsAt(settledAt, false); err != nil {
		return 0, err
	}

	var legacyTransactions []BudgetTransaction
	if err := a.db.Where("task_id = ? AND pending = ?", taskID, true).Find(&legacyTransactions).Error; err != nil {
		return 0, err
	}
	legacyPendingByAccount := make(map[int]int)
	legacyTransactionIDs := make([]int, 0, len(legacyTransactions))
	for _, transaction := range legacyTransactions {
		legacyPendingByAccount[transaction.BudgetAccountID] += transaction.Quota
		legacyTransactionIDs = append(legacyTransactionIDs, transaction.ID)
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if len(legacyTransactionIDs) > 0 {
			if err := tx.Where("id IN ?", legacyTransactionIDs).Delete(&BudgetTransaction{}).Error; err != nil {
				return err
			}
		}
		for accountID, pendingQuota := range legacyPendingByAccount {
			if err := tx.Model(&BudgetAccount{}).Where("id = ?", accountID).Updates(map[string]any{
				"used_quota":    gorm.Expr("used_quota - ?", pendingQuota),
				"pending_quota": gorm.Expr("CASE WHEN pending_quota >= ? THEN pending_quota - ? ELSE 0 END", pendingQuota, pendingQuota),
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&OrganizationUsageLedger{}).
			Where("task_id = ? AND usage_state IN ?", taskID, []string{UsageStatePending, UsageStateSettling}).
			Updates(map[string]any{"created_at": settledAt, "usage_state": UsageStateSettling}).Error
	}); err != nil {
		return 0, err
	}

	disabled := 0
	for accountID := range legacyPendingByAccount {
		var account BudgetAccount
		if err := a.db.First(&account, accountID).Error; err != nil {
			return disabled, err
		}
		if budgetShouldBlock(account, time.Now().Unix()) {
			count, err := a.ensureBudgetBlocks(account, EnterpriseKey{})
			if err != nil {
				return disabled, err
			}
			disabled += count
		} else if _, err := a.releaseBudgetBlocks(account.ID); err != nil {
			return disabled, err
		}
	}
	for i := range ledgers {
		ledgers[i].CreatedAt = settledAt
		ledgers[i].UsageState = UsageStateSettling
		var key EnterpriseKey
		if err := a.db.First(&key, ledgers[i].EnterpriseKeyID).Error; err != nil {
			return disabled, err
		}
		count, err := a.applyBudgetTransactions(ledgers[i], key)
		if err != nil {
			return disabled, err
		}
		disabled += count
	}
	if err := a.db.Model(&OrganizationUsageLedger{}).
		Where("task_id = ? AND usage_state = ?", taskID, UsageStateSettling).
		Update("usage_state", UsageStateSettled).Error; err != nil {
		return disabled, err
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
		Where("usage_state IN ? AND task_id <> ''", []string{UsageStatePending, UsageStateSettling}).
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
		count, err := a.settlePendingTaskBudgetTransactions(item.TaskID, finishedAt)
		if err != nil {
			return disabled, err
		}
		disabled += count
	}
	return disabled, nil
}
