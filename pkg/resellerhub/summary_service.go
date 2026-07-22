package resellerhub

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type dashboardAlert struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	ResellerID   int    `json:"reseller_id,omitempty"`
	ResellerName string `json:"reseller_name,omitempty"`
	Value        int64  `json:"value,omitempty"`
	Limit        int64  `json:"limit,omitempty"`
	TokenID      int    `json:"token_id,omitempty"`
}

type resellerDashboardSummary struct {
	CustomerCount     int64 `json:"customer_count"`
	ActiveKeyCount    int64 `json:"active_key_count"`
	NegativeKeyCount  int64 `json:"negative_key_count"`
	ManagedBalance    int64 `json:"managed_balance"`
	CarrierBalance    int64 `json:"carrier_balance"`
	CarrierTokenCount int64 `json:"carrier_token_count"`
	StaleLedgerCount  int64 `json:"stale_ledger_count"`
}

func (a *App) dashboardSummary(c *gin.Context) {
	identity := currentIdentity(c)
	query := a.db.Model(&Reseller{})
	if identity.HubRole != HubRoleSuperAdmin {
		query = query.Where("id = ?", identity.ResellerID)
	}
	var resellers []Reseller
	if err := query.Order("id").Find(&resellers).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	total := resellerDashboardSummary{}
	alerts := make([]dashboardAlert, 0)
	for _, reseller := range resellers {
		summary, resellerAlerts, err := a.summarizeReseller(reseller)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		total.CustomerCount += summary.CustomerCount
		total.ActiveKeyCount += summary.ActiveKeyCount
		total.NegativeKeyCount += summary.NegativeKeyCount
		total.ManagedBalance += summary.ManagedBalance
		total.CarrierBalance += summary.CarrierBalance
		total.CarrierTokenCount += summary.CarrierTokenCount
		total.StaleLedgerCount += summary.StaleLedgerCount
		alerts = append(alerts, resellerAlerts...)
	}
	respondOK(c, gin.H{
		"reseller_count": len(resellers), "summary": total, "alerts": alerts,
		"max_user_tokens": operation_setting.GetMaxUserTokens(),
	})
}

func (a *App) summarizeReseller(reseller Reseller) (resellerDashboardSummary, []dashboardAlert, error) {
	summary := resellerDashboardSummary{}
	alerts := make([]dashboardAlert, 0)
	if err := a.db.Model(&Customer{}).Where("reseller_id = ?", reseller.ID).Count(&summary.CustomerCount).Error; err != nil {
		return summary, nil, err
	}
	var mappings []CustomerToken
	if err := a.db.Where("reseller_id = ? AND status IN ?", reseller.ID, []string{CustomerTokenStatusActive, CustomerTokenStatusRetiring}).Find(&mappings).Error; err != nil {
		return summary, nil, err
	}
	tokenIDs := make([]int, 0, len(mappings))
	for _, mapping := range mappings {
		tokenIDs = append(tokenIDs, mapping.NewAPITokenID)
		if mapping.Status == CustomerTokenStatusActive {
			summary.ActiveKeyCount++
		}
	}
	if len(tokenIDs) > 0 {
		var tokens []model.Token
		if err := a.db.Select("id", "remain_quota").Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
			return summary, nil, err
		}
		for _, token := range tokens {
			summary.ManagedBalance += int64(token.RemainQuota)
			if token.RemainQuota < 0 {
				summary.NegativeKeyCount++
			}
		}
	}
	var carrier model.User
	if err := a.db.Select("id", "quota").First(&carrier, reseller.QuotaCarrierUserID).Error; err != nil {
		return summary, nil, err
	}
	summary.CarrierBalance = int64(carrier.Quota)
	if err := a.db.Model(&model.Token{}).Where("user_id = ?", reseller.QuotaCarrierUserID).Count(&summary.CarrierTokenCount).Error; err != nil {
		return summary, nil, err
	}
	cutoff := time.Now().Add(-a.config.ConsistencyGrace).Unix()
	if err := a.db.Model(&QuotaLedger{}).
		Where("reseller_id = ? AND status IN ? AND created_at <= ?", reseller.ID, []string{ledgerStatusPrepared, ledgerStatusReconcileRequired}, cutoff).
		Count(&summary.StaleLedgerCount).Error; err != nil {
		return summary, nil, err
	}
	addAlert := func(code, severity string, value, limit int64) {
		alerts = append(alerts, dashboardAlert{Code: code, Severity: severity, ResellerID: reseller.ID, ResellerName: reseller.Name, Value: value, Limit: limit})
	}
	if summary.NegativeKeyCount > 0 {
		addAlert("negative_balance", "danger", summary.NegativeKeyCount, 0)
	}
	if summary.ManagedBalance > summary.CarrierBalance {
		addAlert("managed_exceeds_carrier", "warning", summary.ManagedBalance, summary.CarrierBalance)
	}
	if summary.StaleLedgerCount > 0 {
		addAlert("stale_reconciliation", "danger", summary.StaleLedgerCount, 0)
	}
	if a.config.CarrierLowQuota > 0 && summary.CarrierBalance <= int64(a.config.CarrierLowQuota) {
		addAlert("carrier_low", "warning", summary.CarrierBalance, int64(a.config.CarrierLowQuota))
	}
	if maxTokens := operation_setting.GetMaxUserTokens(); maxTokens > 0 {
		if summary.CarrierTokenCount >= int64(maxTokens) {
			addAlert("key_capacity_blocked", "danger", summary.CarrierTokenCount, int64(maxTokens))
		} else if summary.CarrierTokenCount*100 >= int64(maxTokens)*80 {
			addAlert("key_capacity_warning", "warning", summary.CarrierTokenCount, int64(maxTokens))
		}
	}
	if a.config.KeyQPSAlertThreshold > 0 && len(tokenIDs) > 0 {
		type tokenRequestCount struct {
			TokenID      int   `gorm:"column:token_id"`
			RequestCount int64 `gorm:"column:request_count"`
		}
		var counts []tokenRequestCount
		threshold := int64(a.config.KeyQPSAlertThreshold) * 60
		if err := a.logDB.Model(&model.Log{}).
			Select("token_id, COUNT(*) AS request_count").
			Where("token_id IN ? AND created_at >= ? AND type IN ?", tokenIDs, time.Now().Add(-time.Minute).Unix(), []int{model.LogTypeConsume, model.LogTypeError}).
			Group("token_id").Having("COUNT(*) > ?", threshold).Scan(&counts).Error; err != nil {
			return summary, nil, err
		}
		for _, count := range counts {
			alerts = append(alerts, dashboardAlert{
				Code: "key_qps", Severity: "warning", ResellerID: reseller.ID, ResellerName: reseller.Name,
				TokenID: count.TokenID, Value: (count.RequestCount + 59) / 60, Limit: int64(a.config.KeyQPSAlertThreshold),
			})
		}
	}
	return summary, alerts, nil
}
