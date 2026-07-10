package enterprisepolicyhub

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type policyBudgetAssignment struct {
	Policy    Policy
	ScopeType string
	ScopeID   int
}

func (a *App) ensurePolicyBudgetsAt(timestamp int64, reconcileCurrent bool) error {
	if timestamp <= 0 {
		timestamp = time.Now().Unix()
	}
	var policies []Policy
	if err := a.db.Find(&policies).Error; err != nil {
		return err
	}
	policyByID := make(map[int]Policy, len(policies))
	for _, policy := range policies {
		policyByID[policy.ID] = policy
	}

	assignments := make([]policyBudgetAssignment, 0)
	var orgs []OrgUnit
	if err := a.db.Where("default_policy_id > 0").Find(&orgs).Error; err != nil {
		return err
	}
	for _, org := range orgs {
		if policy, ok := policyByID[org.DefaultPolicyID]; ok {
			assignments = append(assignments, policyBudgetAssignment{Policy: policy, ScopeType: "org_unit", ScopeID: org.ID})
		}
	}
	var keys []EnterpriseKey
	if err := a.db.Where("policy_id > 0").Find(&keys).Error; err != nil {
		return err
	}
	for _, key := range keys {
		if policy, ok := policyByID[key.PolicyID]; ok {
			inherited := false
			if key.OrgUnitID > 0 {
				ancestors, err := a.orgAncestorsRootFirst(key.OrgUnitID)
				if err != nil {
					return err
				}
				for _, org := range ancestors {
					if org.DefaultPolicyID == key.PolicyID {
						inherited = true
						break
					}
				}
			}
			if inherited {
				continue
			}
			assignments = append(assignments, policyBudgetAssignment{Policy: policy, ScopeType: "enterprise_key", ScopeID: key.ID})
		}
	}

	desired := make(map[string]struct{})
	for _, assignment := range assignments {
		if assignment.Policy.Status != StatusEnabled {
			continue
		}
		periods := []struct {
			kind  string
			quota int
		}{
			{kind: BudgetPeriodDaily, quota: assignment.Policy.DailyBudgetQuota},
			{kind: BudgetPeriodMonthly, quota: assignment.Policy.MonthlyBudgetQuota},
		}
		for _, period := range periods {
			if period.quota <= 0 {
				continue
			}
			start, end := a.policyBudgetPeriod(period.kind, timestamp)
			managedKey := policyManagedBudgetKey(assignment.Policy.ID, assignment.ScopeType, assignment.ScopeID, period.kind, start)
			desired[managedKey] = struct{}{}
			var account BudgetAccount
			err := a.db.Where("managed_key = ?", managedKey).First(&account).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				usedQuota, sumErr := a.sumUsageForBudgetScope(assignment.ScopeType, assignment.ScopeID, start, end)
				if sumErr != nil {
					return sumErr
				}
				account = BudgetAccount{
					ScopeType:   assignment.ScopeType,
					ScopeID:     assignment.ScopeID,
					PeriodStart: start,
					PeriodEnd:   end,
					BudgetQuota: period.quota,
					UsedQuota:   usedQuota,
					Currency:    "quota",
					Status:      StatusEnabled,
					SourceType:  BudgetSourcePolicy,
					SourceID:    assignment.Policy.ID,
					PeriodKind:  period.kind,
					ManagedKey:  &managedKey,
				}
				if err := a.db.Create(&account).Error; err != nil {
					if retryErr := a.db.Where("managed_key = ?", managedKey).First(&account).Error; retryErr != nil {
						return err
					}
				} else if err := a.backfillBudgetTransactions(account); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			updates := map[string]any{
				"scope_type":   assignment.ScopeType,
				"scope_id":     assignment.ScopeID,
				"period_start": start,
				"period_end":   end,
				"budget_quota": period.quota,
				"currency":     "quota",
				"status":       StatusEnabled,
				"source_type":  BudgetSourcePolicy,
				"source_id":    assignment.Policy.ID,
				"period_kind":  period.kind,
			}
			if err := a.db.Model(&BudgetAccount{}).Where("id = ?", account.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}

	if reconcileCurrent {
		var currentManaged []BudgetAccount
		if err := a.db.Where("source_type = ? AND status = ?", BudgetSourcePolicy, StatusEnabled).
			Where("(period_start = 0 OR period_start <= ?) AND (period_end = 0 OR period_end > ?)", timestamp, timestamp).
			Find(&currentManaged).Error; err != nil {
			return err
		}
		for _, account := range currentManaged {
			if account.ManagedKey == nil {
				continue
			}
			if _, ok := desired[*account.ManagedKey]; ok {
				continue
			}
			if err := a.db.Model(&BudgetAccount{}).Where("id = ?", account.ID).Update("status", StatusDisabled).Error; err != nil {
				return err
			}
			if _, err := a.releaseBudgetBlocks(account.ID); err != nil {
				return err
			}
		}
		return a.reconcileBudgetEnforcement(timestamp)
	}
	return nil
}

func (a *App) sumUsageForBudgetScope(scopeType string, scopeID int, start int64, end int64) (int, error) {
	query, err := a.budgetLedgerQuery(scopeType, scopeID, start, end)
	if err != nil {
		return 0, err
	}
	var total int64
	if err := query.Select("COALESCE(SUM(quota), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	return int(total), nil
}

func (a *App) budgetLedgerQuery(scopeType string, scopeID int, start int64, end int64) (*gorm.DB, error) {
	query := a.db.Model(&OrganizationUsageLedger{}).
		Where("created_at >= ? AND created_at < ?", start, end)
	switch scopeType {
	case "enterprise_key":
		query = query.Where("enterprise_key_id = ?", scopeID)
	case "org_unit":
		var closures []OrgUnitClosure
		if err := a.db.Where("ancestor_id = ?", scopeID).Find(&closures).Error; err != nil {
			return nil, err
		}
		ids := []int{scopeID}
		if len(closures) > 0 {
			ids = ids[:0]
			for _, closure := range closures {
				ids = append(ids, closure.DescendantID)
			}
		}
		query = query.Where("org_unit_id IN ?", ids)
	case "project":
		query = query.Where("project_id = ?", scopeID)
	case "cost_center":
		query = query.Where("cost_center_id = ?", scopeID)
	default:
		return query.Where("1 = 0"), nil
	}
	return query, nil
}

func (a *App) backfillBudgetTransactions(account BudgetAccount) error {
	query, err := a.budgetLedgerQuery(account.ScopeType, account.ScopeID, account.PeriodStart, account.PeriodEnd)
	if err != nil {
		return err
	}
	var ledgers []OrganizationUsageLedger
	if err := query.Find(&ledgers).Error; err != nil {
		return err
	}
	for _, ledger := range ledgers {
		direction := "consume"
		if ledger.Quota < 0 {
			direction = "refund"
		}
		transaction := BudgetTransaction{
			BudgetAccountID: account.ID,
			EnterpriseKeyID: ledger.EnterpriseKeyID,
			NewAPILogID:     ledger.NewAPILogID,
			SourceType:      "newapi_log",
			SourceID:        ledger.NewAPILogID,
			Quota:           ledger.Quota,
			Direction:       direction,
		}
		if err := a.db.Where("budget_account_id = ? AND new_api_log_id = ?", account.ID, ledger.NewAPILogID).
			FirstOrCreate(&transaction).Error; err != nil {
			return err
		}
	}
	return nil
}

func (a *App) policyBudgetPeriod(kind string, timestamp int64) (int64, int64) {
	location := a.budgetLocation
	if location == nil {
		location = time.UTC
	}
	current := time.Unix(timestamp, 0).In(location)
	var start time.Time
	if kind == BudgetPeriodMonthly {
		start = time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, location)
		return start.Unix(), start.AddDate(0, 1, 0).Unix()
	}
	start = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, location)
	return start.Unix(), start.AddDate(0, 0, 1).Unix()
}

func (a *App) reconcileBudgetEnforcement(now int64) error {
	var activeBlocks []BudgetKeyBlock
	if err := a.db.Where("status = ?", BudgetBlockActive).Find(&activeBlocks).Error; err != nil {
		return err
	}
	accountIDs := make(map[int]struct{}, len(activeBlocks))
	for _, block := range activeBlocks {
		accountIDs[block.BudgetAccountID] = struct{}{}
	}
	for accountID := range accountIDs {
		var account BudgetAccount
		err := a.db.First(&account, accountID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !budgetShouldBlock(account, now) {
			if _, releaseErr := a.releaseBudgetBlocks(accountID); releaseErr != nil {
				return releaseErr
			}
			continue
		}
		if err != nil {
			return err
		}
	}

	var exceeded []BudgetAccount
	if err := a.db.Where("status = ? AND budget_quota > 0 AND used_quota >= budget_quota", StatusEnabled).
		Where("(period_start = 0 OR period_start <= ?) AND (period_end = 0 OR period_end > ?)", now, now).
		Find(&exceeded).Error; err != nil {
		return err
	}
	for _, account := range exceeded {
		if _, err := a.ensureBudgetBlocks(account, EnterpriseKey{}); err != nil {
			return err
		}
	}
	return nil
}

func budgetShouldBlock(account BudgetAccount, now int64) bool {
	if account.Status != StatusEnabled || account.BudgetQuota <= 0 || account.UsedQuota < account.BudgetQuota {
		return false
	}
	if account.PeriodStart > 0 && account.PeriodStart > now {
		return false
	}
	return account.PeriodEnd <= 0 || account.PeriodEnd > now
}

func (a *App) budgetTargetKeys(account BudgetAccount, fallback EnterpriseKey) ([]EnterpriseKey, error) {
	var keys []EnterpriseKey
	switch account.ScopeType {
	case "enterprise_key":
		if err := a.db.Where("id = ?", account.ScopeID).Find(&keys).Error; err != nil {
			return nil, err
		}
	case "org_unit":
		var closures []OrgUnitClosure
		if err := a.db.Where("ancestor_id = ?", account.ScopeID).Find(&closures).Error; err != nil {
			return nil, err
		}
		ids := make([]int, 0, len(closures))
		for _, closure := range closures {
			ids = append(ids, closure.DescendantID)
		}
		if len(ids) > 0 {
			if err := a.db.Where("org_unit_id IN ?", ids).Find(&keys).Error; err != nil {
				return nil, err
			}
		}
	case "project":
		if err := a.db.Where("project_id = ?", account.ScopeID).Find(&keys).Error; err != nil {
			return nil, err
		}
	case "cost_center":
		if err := a.db.Where("cost_center_id = ?", account.ScopeID).Find(&keys).Error; err != nil {
			return nil, err
		}
	default:
		if fallback.ID > 0 {
			keys = []EnterpriseKey{fallback}
		}
	}
	return keys, nil
}

func (a *App) ensureBudgetBlocks(account BudgetAccount, fallback EnterpriseKey) (int, error) {
	keys, err := a.budgetTargetKeys(account, fallback)
	if err != nil {
		return 0, err
	}
	blocked := 0
	for _, key := range keys {
		changed := false
		var block BudgetKeyBlock
		err := a.db.Where("budget_account_id = ? AND enterprise_key_id = ?", account.ID, key.ID).First(&block).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			block = BudgetKeyBlock{BudgetAccountID: account.ID, EnterpriseKeyID: key.ID, Status: BudgetBlockActive}
			if err := a.db.Create(&block).Error; err != nil {
				return blocked, err
			}
			blocked++
			changed = true
		} else if err != nil {
			return blocked, err
		} else if block.Status != BudgetBlockActive {
			if err := a.db.Model(&BudgetKeyBlock{}).Where("id = ?", block.ID).Updates(map[string]any{
				"status":      BudgetBlockActive,
				"released_at": 0,
			}).Error; err != nil {
				return blocked, err
			}
			blocked++
			changed = true
		}
		if !changed {
			continue
		}
		if err := a.db.Model(&EnterpriseKey{}).Where("id = ?", key.ID).Update("sync_status", StatusPending).Error; err != nil {
			return blocked, err
		}
		if _, err := a.syncEnterpriseKey(key.ID, false); err != nil {
			return blocked, err
		}
	}
	return blocked, nil
}

func (a *App) releaseBudgetBlocks(accountID int) (int, error) {
	var blocks []BudgetKeyBlock
	if err := a.db.Where("budget_account_id = ? AND status = ?", accountID, BudgetBlockActive).Find(&blocks).Error; err != nil {
		return 0, err
	}
	if len(blocks) == 0 {
		return 0, nil
	}
	keyIDs := make(map[int]struct{}, len(blocks))
	for _, block := range blocks {
		keyIDs[block.EnterpriseKeyID] = struct{}{}
	}
	if err := a.db.Model(&BudgetKeyBlock{}).
		Where("budget_account_id = ? AND status = ?", accountID, BudgetBlockActive).
		Updates(map[string]any{"status": BudgetBlockReleased, "released_at": time.Now().Unix()}).Error; err != nil {
		return 0, err
	}
	for keyID := range keyIDs {
		if err := a.db.Model(&EnterpriseKey{}).Where("id = ?", keyID).Update("sync_status", StatusPending).Error; err != nil {
			return 0, err
		}
		if _, err := a.syncEnterpriseKey(keyID, false); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, err
		}
	}
	return len(blocks), nil
}

func (a *App) activeBudgetBlockCount(keyID int) (int64, error) {
	var count int64
	err := a.db.Model(&BudgetKeyBlock{}).
		Where("enterprise_key_id = ? AND status = ?", keyID, BudgetBlockActive).
		Count(&count).Error
	return count, err
}

func policyManagedBudgetKey(policyID int, scopeType string, scopeID int, periodKind string, periodStart int64) string {
	return "policy:" + strconv.Itoa(policyID) + ":" + scopeType + ":" + strconv.Itoa(scopeID) + ":" + periodKind + ":" + strconv.FormatInt(periodStart, 10)
}

func (a *App) syncPendingEnterpriseKeys(limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var keys []EnterpriseKey
	if err := a.db.Where("sync_status = ?", StatusPending).Order("id asc").Limit(limit).Find(&keys).Error; err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := a.syncEnterpriseKey(key.ID, false); err != nil {
			common.SysError("enterprise key " + strconv.Itoa(key.ID) + " sync failed: " + err.Error())
		}
	}
	return nil
}
