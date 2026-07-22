package resellerhub

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type usageItem struct {
	LogID            int    `json:"log_id"`
	CreatedAt        int64  `json:"created_at"`
	TokenID          int    `json:"token_id"`
	TokenName        string `json:"token_name"`
	ModelName        string `json:"model_name"`
	ChannelID        int    `json:"channel_id"`
	LogType          int    `json:"log_type"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	SignedQuota      int    `json:"signed_quota"`
	Quota            int    `json:"quota"`
	DiscountBPS      int    `json:"discount_bps"`
	StandardAmount   string `json:"standard_amount"`
	DiscountedAmount string `json:"discounted_amount"`
	RequestID        string `json:"request_id"`
}

func (a *App) customerUsage(c *gin.Context) {
	customerID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	customer, ok := a.customerForRequest(c, customerID)
	if !ok {
		return
	}
	var mappings []CustomerToken
	if err := a.db.Where("reseller_id = ? AND customer_id = ?", customer.ResellerID, customer.ID).Find(&mappings).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	tokenIDs := make([]int, 0, len(mappings))
	for _, mapping := range mappings {
		tokenIDs = append(tokenIDs, mapping.NewAPITokenID)
	}
	if len(tokenIDs) == 0 {
		respondOK(c, gin.H{"items": []usageItem{}, "summary": gin.H{"standard_quota": 0, "discounted_amount": "0"}})
		return
	}
	query, ok := a.scopedUsageQuery(c, tokenIDs)
	if !ok {
		return
	}
	summaryQuery, ok := a.scopedUsageQuery(c, tokenIDs)
	if !ok {
		return
	}
	page := queryPositiveInt(c, "page", 1, 1000000)
	pageSize := queryPositiveInt(c, "page_size", 50, 200)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var logs []model.Log
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	currency, err := a.fetchCurrencyConfig(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	var versions []DiscountVersion
	if err := a.db.Where("reseller_id = ? AND customer_id IN ?", customer.ResellerID, []int{0, customer.ID}).Order("effective_at").Find(&versions).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var reseller Reseller
	if err := a.db.First(&reseller, customer.ResellerID).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]usageItem, 0, len(logs))
	for _, log := range logs {
		signedQuota := log.Quota
		if log.Type == model.LogTypeRefund {
			signedQuota = -signedQuota
		}
		discount := discountAt(versions, customer.ID, log.CreatedAt, reseller.DefaultDiscountBPS)
		standard, discounted := quotaAmounts(signedQuota, currency, discount)
		items = append(items, usageItem{
			LogID: log.Id, CreatedAt: log.CreatedAt, TokenID: log.TokenId, TokenName: log.TokenName,
			ModelName: log.ModelName, ChannelID: log.ChannelId, LogType: log.Type,
			PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens,
			SignedQuota: signedQuota, Quota: signedQuota, DiscountBPS: discount,
			StandardAmount: standard.StringFixed(6), DiscountedAmount: discounted.StringFixed(6), RequestID: log.RequestId,
		})
	}
	standardQuotaTotal := 0
	discountedAmountTotal := decimal.Zero
	standardAmountTotal := decimal.Zero
	rows, err := summaryQuery.Select("created_at", "type", "quota").Rows()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var createdAt int64
		var logType, quota int
		if err = rows.Scan(&createdAt, &logType, &quota); err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		if logType == model.LogTypeRefund {
			quota = -quota
		}
		discount := discountAt(versions, customer.ID, createdAt, reseller.DefaultDiscountBPS)
		standard, discounted := quotaAmounts(quota, currency, discount)
		standardQuotaTotal += quota
		standardAmountTotal = standardAmountTotal.Add(standard)
		discountedAmountTotal = discountedAmountTotal.Add(discounted)
	}
	respondOK(c, gin.H{
		"items": items, "total": total, "page": page, "page_size": pageSize,
		"summary": gin.H{
			"standard_quota":    standardQuotaTotal,
			"standard_amount":   standardAmountTotal.StringFixed(6),
			"discounted_amount": discountedAmountTotal.StringFixed(6),
			"currency":          currencyView(currency),
		},
	})
}

func (a *App) scopedUsageQuery(c *gin.Context, tokenIDs []int) (*gorm.DB, bool) {
	query := a.logDB.Model(&model.Log{}).Where("token_id IN ? AND type IN ?", tokenIDs, []int{model.LogTypeConsume, model.LogTypeRefund})
	if tokenID, err := strconv.Atoi(c.Query("token_id")); err == nil && tokenID > 0 {
		owned := false
		for _, id := range tokenIDs {
			if id == tokenID {
				owned = true
				break
			}
		}
		if !owned {
			respondError(c, http.StatusNotFound, "token not found")
			return nil, false
		}
		query = query.Where("token_id = ?", tokenID)
	}
	if modelName := strings.TrimSpace(c.Query("model")); modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	fromValue := c.Query("from")
	if fromValue == "" {
		fromValue = c.Query("start_time")
	}
	if from, err := parseTimeFilter(fromValue); err != nil {
		respondError(c, http.StatusBadRequest, "invalid start_time")
		return nil, false
	} else if from > 0 {
		query = query.Where("created_at >= ?", from)
	}
	toValue := c.Query("to")
	if toValue == "" {
		toValue = c.Query("end_time")
	}
	if to, err := parseTimeFilter(toValue); err != nil {
		respondError(c, http.StatusBadRequest, "invalid end_time")
		return nil, false
	} else if to > 0 {
		query = query.Where("created_at <= ?", to)
	}
	return query, true
}

func parseTimeFilter(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02T15:04", value, time.Local)
	if err != nil {
		return 0, err
	}
	return parsed.Unix(), nil
}

func discountAt(versions []DiscountVersion, customerID int, timestamp int64, fallback int) int {
	selected := fallback
	selectedAt := int64(-1)
	selectedScope := 0
	for _, version := range versions {
		if version.EffectiveAt > timestamp || (version.EndedAt != nil && *version.EndedAt <= timestamp) {
			continue
		}
		scope := 1
		if version.CustomerID == customerID {
			scope = 2
		} else if version.CustomerID != 0 {
			continue
		}
		if scope > selectedScope || (scope == selectedScope && version.EffectiveAt >= selectedAt) {
			selected = version.DiscountBPS
			selectedAt = version.EffectiveAt
			selectedScope = scope
		}
	}
	return selected
}

func sortedTokenIDs(mappings []CustomerToken) []int {
	ids := make([]int, 0, len(mappings))
	for _, mapping := range mappings {
		ids = append(ids, mapping.NewAPITokenID)
	}
	sort.Ints(ids)
	return ids
}
