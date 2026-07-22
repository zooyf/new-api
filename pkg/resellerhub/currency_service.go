package resellerhub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type CurrencyConfig struct {
	DisplayType      string          `json:"display_type"`
	Symbol           string          `json:"symbol"`
	QuotaPerUnit     decimal.Decimal `json:"quota_per_unit"`
	USDToDisplayRate decimal.Decimal `json:"usd_to_display_rate"`
	Version          string          `json:"version"`
}

type currencyConfigView struct {
	DisplayType      string `json:"display_type"`
	Symbol           string `json:"symbol"`
	QuotaPerUnit     string `json:"quota_per_unit"`
	USDToDisplayRate string `json:"usd_to_display_rate"`
	Version          string `json:"version"`
}

type gatewayStatusResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		QuotaDisplayType           string  `json:"quota_display_type"`
		QuotaPerUnit               float64 `json:"quota_per_unit"`
		USDExchangeRate            float64 `json:"usd_exchange_rate"`
		CustomCurrencySymbol       string  `json:"custom_currency_symbol"`
		CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
	} `json:"data"`
}

func (a *App) fetchCurrencyConfig(ctx context.Context) (CurrencyConfig, error) {
	if a.config.GatewayBaseURL == "" {
		return CurrencyConfig{}, errors.New("RESELLER_HUB_GATEWAY_BASE_URL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.GatewayBaseURL+"/api/status", nil)
	if err != nil {
		return CurrencyConfig{}, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return CurrencyConfig{}, fmt.Errorf("read gateway currency config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CurrencyConfig{}, fmt.Errorf("gateway status returned HTTP %d", resp.StatusCode)
	}
	var payload gatewayStatusResponse
	if err = common.DecodeJson(resp.Body, &payload); err != nil {
		return CurrencyConfig{}, fmt.Errorf("decode gateway status: %w", err)
	}
	if !payload.Success {
		return CurrencyConfig{}, errors.New("gateway status is unavailable")
	}
	displayType := strings.ToUpper(strings.TrimSpace(payload.Data.QuotaDisplayType))
	if displayType == "" {
		displayType = "USD"
	}
	quotaPerUnit := decimal.NewFromFloat(payload.Data.QuotaPerUnit)
	if !quotaPerUnit.IsPositive() {
		return CurrencyConfig{}, errors.New("gateway quota_per_unit must be positive")
	}
	symbol := "$"
	rate := decimal.NewFromInt(1)
	switch displayType {
	case "USD":
	case "CNY":
		symbol = "¥"
		rate = decimal.NewFromFloat(payload.Data.USDExchangeRate)
	case "CUSTOM":
		symbol = strings.TrimSpace(payload.Data.CustomCurrencySymbol)
		if symbol == "" {
			return CurrencyConfig{}, errors.New("custom currency symbol is empty")
		}
		rate = decimal.NewFromFloat(payload.Data.CustomCurrencyExchangeRate)
	case "TOKENS":
		symbol = ""
		rate = decimal.NewFromInt(1)
	default:
		return CurrencyConfig{}, errors.New("unsupported quota display type")
	}
	if !rate.IsPositive() {
		return CurrencyConfig{}, errors.New("gateway currency rate must be positive")
	}
	versionInput := strings.Join([]string{displayType, symbol, quotaPerUnit.String(), rate.String()}, "|")
	digest := sha256.Sum256([]byte(versionInput))
	return CurrencyConfig{
		DisplayType:      displayType,
		Symbol:           symbol,
		QuotaPerUnit:     quotaPerUnit,
		USDToDisplayRate: rate,
		Version:          hex.EncodeToString(digest[:8]),
	}, nil
}

func currencyView(config CurrencyConfig) currencyConfigView {
	return currencyConfigView{
		DisplayType:      config.DisplayType,
		Symbol:           config.Symbol,
		QuotaPerUnit:     config.QuotaPerUnit.String(),
		USDToDisplayRate: config.USDToDisplayRate.String(),
		Version:          config.Version,
	}
}

func (a *App) quotaConversionConfig(c *gin.Context) {
	config, err := a.fetchCurrencyConfig(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	respondOK(c, currencyView(config))
}

func quotaFromInput(inputUnit, amount string, config CurrencyConfig, discountBPS int) (int, decimal.Decimal, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil || !value.IsPositive() {
		return 0, decimal.Zero, errors.New("amount must be a positive decimal")
	}
	switch inputUnit {
	case "quota":
		if !value.Equal(value.Truncate(0)) {
			return 0, decimal.Zero, errors.New("quota amount must be an integer")
		}
		quota, clamp := common.QuotaFromDecimalChecked(value)
		if clamp != nil || quota <= 0 {
			return 0, decimal.Zero, errors.New("quota amount is outside the supported range")
		}
		standardDisplay := value.Div(config.QuotaPerUnit).Mul(config.USDToDisplayRate)
		return quota, standardDisplay.Mul(decimal.NewFromInt(int64(discountBPS))).Div(decimal.NewFromInt(10000)), nil
	case "display_currency":
		if config.DisplayType == "TOKENS" {
			return 0, decimal.Zero, errors.New("display currency input is unavailable in TOKENS mode")
		}
		if discountBPS <= 0 {
			return 0, decimal.Zero, errors.New("discount must be positive")
		}
		discount := decimal.NewFromInt(int64(discountBPS)).Div(decimal.NewFromInt(10000))
		standardUSD := value.Div(config.USDToDisplayRate).Div(discount)
		quotaDecimal := standardUSD.Mul(config.QuotaPerUnit)
		quota, clamp := common.QuotaFromDecimalChecked(quotaDecimal)
		if clamp != nil || quota <= 0 {
			return 0, decimal.Zero, errors.New("converted quota is outside the supported range")
		}
		return quota, value, nil
	default:
		return 0, decimal.Zero, errors.New("input_unit must be quota or display_currency")
	}
}

func quotaAmounts(quota int, config CurrencyConfig, discountBPS int) (standard, discounted decimal.Decimal) {
	standard = decimal.NewFromInt(int64(quota)).Div(config.QuotaPerUnit).Mul(config.USDToDisplayRate)
	discounted = standard.Mul(decimal.NewFromInt(int64(discountBPS))).Div(decimal.NewFromInt(10000))
	return standard, discounted
}
