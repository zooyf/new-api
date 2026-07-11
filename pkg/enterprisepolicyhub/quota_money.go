package enterprisepolicyhub

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

const quotaCurrency = "QUOTA"

type monetaryQuotaInput struct {
	Amount          *decimal.Decimal
	Currency        string
	Unlimited       bool
	LegacyQuota     int
	RequirePositive bool
}

type monetaryQuotaSnapshot struct {
	Quota        int
	Amount       string
	Currency     string
	QuotaPerUnit float64
	ExchangeRate float64
}

type quotaCurrencyConfig struct {
	QuotaPerUnit               float64 `json:"quota_per_unit"`
	DisplayType                string  `json:"display_type"`
	CurrencySymbol             string  `json:"currency_symbol"`
	USDExchangeRate            float64 `json:"usd_exchange_rate"`
	CustomCurrencySymbol       string  `json:"custom_currency_symbol"`
	CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
}

func currentQuotaCurrencyConfig() quotaCurrencyConfig {
	general := operation_setting.GetGeneralSetting()
	return quotaCurrencyConfig{
		QuotaPerUnit:               common.QuotaPerUnit,
		DisplayType:                operation_setting.GetQuotaDisplayType(),
		CurrencySymbol:             operation_setting.GetCurrencySymbol(),
		USDExchangeRate:            operation_setting.USDExchangeRate,
		CustomCurrencySymbol:       general.CustomCurrencySymbol,
		CustomCurrencyExchangeRate: general.CustomCurrencyExchangeRate,
	}
}

func resolveMonetaryQuota(input monetaryQuotaInput) (monetaryQuotaSnapshot, error) {
	currency := normalizeQuotaCurrency(input.Currency)
	if strings.TrimSpace(input.Currency) != "" && currency == "" {
		return monetaryQuotaSnapshot{}, fmt.Errorf("unsupported currency %q", input.Currency)
	}
	if currency == "" {
		currency = normalizeQuotaCurrency(operation_setting.GetQuotaDisplayType())
	}
	if currency == "" {
		currency = operation_setting.QuotaDisplayTypeUSD
	}
	if input.Unlimited {
		return monetaryQuotaSnapshot{
			Amount:       "0",
			Currency:     currency,
			QuotaPerUnit: common.QuotaPerUnit,
			ExchangeRate: quotaExchangeRate(currency),
		}, nil
	}
	if input.Amount == nil {
		if input.RequirePositive && input.LegacyQuota <= 0 {
			return monetaryQuotaSnapshot{}, errors.New("quota must be greater than zero")
		}
		return monetaryQuotaSnapshot{Quota: input.LegacyQuota}, nil
	}
	if input.Amount.IsNegative() {
		return monetaryQuotaSnapshot{}, errors.New("amount must not be negative")
	}
	if input.RequirePositive && !input.Amount.IsPositive() {
		return monetaryQuotaSnapshot{}, errors.New("amount must be greater than zero")
	}

	rate := quotaExchangeRate(currency)
	if rate <= 0 {
		return monetaryQuotaSnapshot{}, fmt.Errorf("exchange rate for %s must be greater than zero", currency)
	}
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quotaDecimal := input.Amount.Mul(quotaPerUnit).Div(decimal.NewFromFloat(rate)).Round(0)
	maxInt := int64(^uint(0) >> 1)
	if quotaDecimal.GreaterThan(decimal.NewFromInt(maxInt)) {
		return monetaryQuotaSnapshot{}, errors.New("amount is too large")
	}
	quota := quotaDecimal.IntPart()
	return monetaryQuotaSnapshot{
		Quota:        int(quota),
		Amount:       input.Amount.String(),
		Currency:     currency,
		QuotaPerUnit: common.QuotaPerUnit,
		ExchangeRate: rate,
	}, nil
}

func normalizeQuotaCurrency(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case operation_setting.QuotaDisplayTypeUSD:
		return operation_setting.QuotaDisplayTypeUSD
	case operation_setting.QuotaDisplayTypeCNY:
		return operation_setting.QuotaDisplayTypeCNY
	case operation_setting.QuotaDisplayTypeCustom:
		return operation_setting.QuotaDisplayTypeCustom
	case operation_setting.QuotaDisplayTypeTokens, quotaCurrency:
		return quotaCurrency
	default:
		return ""
	}
}

func quotaExchangeRate(currency string) float64 {
	switch normalizeQuotaCurrency(currency) {
	case operation_setting.QuotaDisplayTypeCNY:
		return operation_setting.USDExchangeRate
	case operation_setting.QuotaDisplayTypeCustom:
		return operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
	case quotaCurrency:
		return common.QuotaPerUnit
	default:
		return 1
	}
}
