package doubao

import (
	"net/http/httptest"
	"testing"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	seedancepricing "github.com/QuantumNous/new-api/setting/seedance_video_pricing"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func estimateCNYBilling(t *testing.T, modelName string, resolution string, hasVideo bool, groupRatio float64, duration int) *channel.TaskBillingEstimate {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	metadata := map[string]interface{}{}
	if hasVideo {
		metadata["content"] = []interface{}{
			map[string]interface{}{"type": "video_url", "video_url": map[string]interface{}{"url": "https://example.com/reference.mp4"}},
		}
	}
	request := relaycommon.TaskSubmitReq{
		Prompt:     "test",
		Model:      modelName,
		Resolution: resolution,
		Duration:   duration,
		Metadata:   metadata,
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDoubaoVideo},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: modelName,
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: groupRatio},
		},
	}
	relaycommon.StoreTaskRequest(ctx, info, constant.TaskActionGenerate, request)
	estimate, taskErr := (&TaskAdaptor{}).EstimateTaskBilling(ctx, info)
	require.Nil(t, taskErr)
	require.NotNil(t, estimate)
	return estimate
}

func replaceCNYPricesForTest(t *testing.T, prices seedancepricing.PricesCNY) {
	t.Helper()
	var original string
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if key == seedancepricing.OptionKey {
			original = value
		}
		return nil
	}))
	require.NotEmpty(t, original)
	t.Cleanup(func() {
		cfg := config.GlobalConfig.Get("seedance_video_pricing")
		require.NotNil(t, cfg)
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{"prices_cny": original}))
		require.NoError(t, seedancepricing.RebuildPriceIndex())
	})

	encoded, err := appcommon.Marshal(prices)
	require.NoError(t, err)
	cfg := config.GlobalConfig.Get("seedance_video_pricing")
	require.NotNil(t, cfg)
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{"prices_cny": string(encoded)}))
	require.NoError(t, seedancepricing.RebuildPriceIndex())
}

func TestEstimateTaskBillingUsesConfiguredCNYMatrix(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	oldExchangeRate := operation_setting.USDExchangeRate
	appcommon.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	tests := []struct {
		name           string
		model          string
		resolution     string
		hasVideo       bool
		priceCNY       int64
		tokens         int64
		wantResolution string
	}{
		{name: "standard 720p", model: seedancepricing.StandardSeedanceModel, resolution: "720p", priceCNY: 46, tokens: 108_000, wantResolution: "720p"},
		{name: "standard 720p with video", model: seedancepricing.StandardSeedanceModel, resolution: "720p", hasVideo: true, priceCNY: 28, tokens: 432_000, wantResolution: "720p"},
		{name: "standard 1080p", model: seedancepricing.StandardSeedanceModel, resolution: "1080p", priceCNY: 51, tokens: 243_000, wantResolution: "1080p"},
		{name: "standard 1080p with video", model: seedancepricing.StandardSeedanceModel, resolution: "1080p", hasVideo: true, priceCNY: 31, tokens: 972_000, wantResolution: "1080p"},
		{name: "standard 4k", model: seedancepricing.StandardSeedanceModel, resolution: "4K", priceCNY: 26, tokens: 972_000, wantResolution: "4k"},
		{name: "standard 4k with video", model: seedancepricing.StandardSeedanceModel, resolution: "4K", hasVideo: true, priceCNY: 16, tokens: 3_888_000, wantResolution: "4k"},
		{name: "fast default", model: seedancepricing.FastSeedanceModel, resolution: "1080p", priceCNY: 37, tokens: 243_000, wantResolution: "1080p"},
		{name: "fast default with video", model: seedancepricing.FastSeedanceModel, resolution: "4K", hasVideo: true, priceCNY: 22, tokens: 3_888_000, wantResolution: "4k"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimate := estimateCNYBilling(t, tt.model, tt.resolution, tt.hasVideo, 1, 5)
			snapshot := estimate.Snapshot
			require.NotNil(t, snapshot)
			assert.Equal(t, model.TaskBillingProviderDoubaoVideoCNY, snapshot.Provider)
			assert.Equal(t, "CNY", snapshot.Currency)
			assert.Equal(t, decimal.NewFromInt(tt.priceCNY).String(), snapshot.UnitPricePerMillionTokens)
			assert.Equal(t, tt.hasVideo, snapshot.HasVideoInput)
			assert.Equal(t, tt.tokens, snapshot.EstimatedTokens)
			assert.Equal(t, tt.wantResolution, snapshot.Resolution)
			expected := decimal.NewFromInt(tt.tokens).
				Div(decimal.NewFromInt(1_000_000)).
				Mul(decimal.NewFromInt(tt.priceCNY)).
				Div(decimal.NewFromFloat(7.3)).
				Mul(decimal.NewFromFloat(appcommon.QuotaPerUnit))
			assert.Equal(t, appcommon.QuotaFromDecimal(expected), estimate.PriceData.Quota)
			assert.False(t, estimate.PriceData.UsePrice)
		})
	}
}

func TestEstimateAndSettlementFreezeConfiguredGroupRatio(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	oldExchangeRate := operation_setting.USDExchangeRate
	appcommon.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	tests := []struct {
		name               string
		userGroup          string
		groupRatioJSON     string
		specialRatioJSON   string
		expectedRatio      float64
		expectedSpecial    bool
		changedGroupJSON   string
		changedSpecialJSON string
	}{
		{
			name:               "ordinary group ratio",
			userGroup:          "ordinary-user",
			groupRatioJSON:     `{"seedance-test":1.2}`,
			specialRatioJSON:   `{}`,
			expectedRatio:      1.2,
			changedGroupJSON:   `{"seedance-test":9}`,
			changedSpecialJSON: `{}`,
		},
		{
			name:               "user group special ratio",
			userGroup:          "vip-user",
			groupRatioJSON:     `{"seedance-test":1.2}`,
			specialRatioJSON:   `{"vip-user":{"seedance-test":1.35}}`,
			expectedRatio:      1.35,
			expectedSpecial:    true,
			changedGroupJSON:   `{"seedance-test":9}`,
			changedSpecialJSON: `{"vip-user":{"seedance-test":8}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalGroupRatios := ratio_setting.GroupRatio2JSONString()
			originalSpecialRatios := ratio_setting.GroupGroupRatio2JSONString()
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
				require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalSpecialRatios))
			})
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(tt.groupRatioJSON))
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(tt.specialRatioJSON))

			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			request := relaycommon.TaskSubmitReq{
				Prompt:     "test",
				Model:      seedancepricing.StandardSeedanceModel,
				Resolution: "720p",
				Duration:   5,
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDoubaoVideo},
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
				OriginModelName: seedancepricing.StandardSeedanceModel,
				UserGroup:       tt.userGroup,
				UsingGroup:      "seedance-test",
			}
			info.PriceData.GroupRatioInfo = relayhelper.HandleGroupRatio(ctx, info)
			assert.Equal(t, tt.expectedRatio, info.PriceData.GroupRatioInfo.GroupRatio)
			assert.Equal(t, tt.expectedSpecial, info.PriceData.GroupRatioInfo.HasSpecialRatio)
			relaycommon.StoreTaskRequest(ctx, info, constant.TaskActionGenerate, request)

			estimate, taskErr := (&TaskAdaptor{}).EstimateTaskBilling(ctx, info)
			require.Nil(t, taskErr)
			require.NotNil(t, estimate)
			require.NotNil(t, estimate.Snapshot)
			assert.Equal(t, tt.expectedRatio, estimate.Snapshot.GroupRatio)

			baseQuota := decimal.NewFromInt(108_000).
				Div(decimal.NewFromInt(1_000_000)).
				Mul(decimal.NewFromInt(46)).
				Div(decimal.NewFromFloat(7.3)).
				Mul(decimal.NewFromInt(500_000))
			expectedEstimate := baseQuota.Mul(decimal.NewFromFloat(tt.expectedRatio))
			assert.Equal(t, appcommon.QuotaFromDecimal(expectedEstimate), estimate.PriceData.Quota)
			assert.InDelta(
				t,
				float64(appcommon.QuotaFromDecimal(baseQuota))*tt.expectedRatio,
				float64(estimate.PriceData.Quota),
				1,
			)

			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(tt.changedGroupJSON))
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(tt.changedSpecialJSON))
			task := &model.Task{}
			task.PrivateData.BillingContext = &model.TaskBillingContext{
				GroupRatio:      9,
				ProviderBilling: estimate.Snapshot,
			}
			quota, clamp, handled, err := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(
				task,
				&relaycommon.TaskInfo{TotalTokens: 1_000_000},
			)
			require.NoError(t, err)
			require.True(t, handled)
			assert.Nil(t, clamp)
			expectedSettlement := decimal.NewFromInt(46).
				Div(decimal.NewFromFloat(7.3)).
				Mul(decimal.NewFromInt(500_000)).
				Mul(decimal.NewFromFloat(tt.expectedRatio))
			assert.Equal(t, appcommon.QuotaFromDecimal(expectedSettlement), quota)
		})
	}
}

func TestAdjustBillingOnCompleteUsesFrozenCNYPriceExchangeAndGroup(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	oldExchangeRate := operation_setting.USDExchangeRate
	appcommon.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	estimate := estimateCNYBilling(t, seedancepricing.StandardSeedanceModel, "720p", false, 1.2, 5)
	prices := seedancepricing.DefaultPricesCNY()
	prices[seedancepricing.StandardSeedanceModel]["720p"][seedancepricing.WithoutVideoKey] = 99
	replaceCNYPricesForTest(t, prices)
	operation_setting.USDExchangeRate = 9.9

	task := &model.Task{}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		GroupRatio:      9,
		ProviderBilling: estimate.Snapshot,
	}
	quota, clamp, handled, err := (&TaskAdaptor{}).AdjustBillingOnCompleteChecked(
		task,
		&relaycommon.TaskInfo{TotalTokens: 1_000_000},
	)

	require.NoError(t, err)
	require.True(t, handled)
	assert.Nil(t, clamp)
	expected := decimal.NewFromInt(46).
		Div(decimal.NewFromFloat(7.3)).
		Mul(decimal.NewFromFloat(500_000)).
		Mul(decimal.NewFromFloat(1.2))
	assert.Equal(t, appcommon.QuotaFromDecimal(expected), quota)
}

func TestEstimateTaskBillingRejectsMetadataDurationBypass(t *testing.T) {
	oldExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := relaycommon.TaskSubmitReq{
		Prompt: "test",
		Model:  seedancepricing.StandardSeedanceModel,
		Metadata: map[string]interface{}{
			"duration": relaycommon.MaxTaskDurationSeconds + 1,
		},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDoubaoVideo},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: seedancepricing.StandardSeedanceModel,
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
	}
	relaycommon.StoreTaskRequest(ctx, info, constant.TaskActionGenerate, request)

	estimate, taskErr := (&TaskAdaptor{}).EstimateTaskBilling(ctx, info)

	assert.Nil(t, estimate)
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)
}

func TestEstimateTaskBillingSurfacesQuotaClamp(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	oldExchangeRate := operation_setting.USDExchangeRate
	appcommon.QuotaPerUnit = 500_000
	operation_setting.USDExchangeRate = 7.3
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := relaycommon.TaskSubmitReq{
		Prompt:     "test",
		Model:      seedancepricing.StandardSeedanceModel,
		Resolution: "4k",
		Duration:   relaycommon.MaxTaskDurationSeconds,
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeDoubaoVideo},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		OriginModelName: seedancepricing.StandardSeedanceModel,
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
		},
	}
	relaycommon.StoreTaskRequest(ctx, info, constant.TaskActionGenerate, request)

	estimate, taskErr := (&TaskAdaptor{}).EstimateTaskBilling(ctx, info)

	require.Nil(t, taskErr)
	require.NotNil(t, estimate)
	require.NotNil(t, info.QuotaClamp)
	assert.Equal(t, appcommon.QuotaClampOverflow, info.QuotaClamp.Kind)
	assert.Equal(t, appcommon.MaxQuota, estimate.PriceData.Quota)
}

func TestSupportsTaskBillingIsModelAndChannelScoped(t *testing.T) {
	adaptor := &TaskAdaptor{}
	assert.True(t, adaptor.SupportsTaskBilling(constant.ChannelTypeDoubaoVideo, seedancepricing.StandardSeedanceModel))
	assert.True(t, adaptor.SupportsTaskBilling(constant.ChannelTypeDoubaoVideo, seedancepricing.FastSeedanceModel))
	assert.False(t, adaptor.SupportsTaskBilling(constant.ChannelTypeVolcEngine, seedancepricing.StandardSeedanceModel))
	assert.False(t, adaptor.SupportsTaskBilling(constant.ChannelTypeDoubaoVideo, "doubao-seedance-2-0-filter-off"))
	assert.False(t, adaptor.SupportsTaskBilling(constant.ChannelTypeDoubaoVideo, "dreamina-seedance-2-0-mini-filter-off"))
}

func TestAdjustBillingOnCompleteUsesOfficialPriceTable(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	appcommon.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
	})

	task := &model.Task{}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "doubao-seedance-2-0-filter-off",
		ModelRatio:      999,
		GroupRatio:      1,
		OtherRatios: map[string]float64{
			videoInputRatioKey: 4.3 / 7.0,
		},
	}
	taskResult := &relaycommon.TaskInfo{TotalTokens: 1_000_000}

	got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, taskResult)

	require.Positive(t, got)
	assert.Equal(t, 2_150_000, got)
}

func TestAdjustBillingOnCompleteAppliesGroupRatio(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	appcommon.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
	})

	task := &model.Task{}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "doubao-seedance-2-0-fast-filter-off",
		GroupRatio:      0.5,
	}
	taskResult := &relaycommon.TaskInfo{TotalTokens: 1_000_000}

	got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, taskResult)

	assert.Equal(t, 1_400_000, got)
}

func TestAdjustBillingOnCompleteRoundsSupplierUsageExactly(t *testing.T) {
	oldQuotaPerUnit := appcommon.QuotaPerUnit
	appcommon.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		appcommon.QuotaPerUnit = oldQuotaPerUnit
	})

	task := &model.Task{}
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "doubao-seedance-2-0-fast-filter-off",
		GroupRatio:      1,
	}
	taskResult := &relaycommon.TaskInfo{TotalTokens: 48_400}

	got := (&TaskAdaptor{}).AdjustBillingOnComplete(task, taskResult)

	assert.Equal(t, 135_520, got)
}

func TestAdjustBillingOnCompleteFallsBackForUnsupportedCases(t *testing.T) {
	tests := []struct {
		name       string
		task       *model.Task
		taskResult *relaycommon.TaskInfo
	}{
		{
			name: "nil task",
		},
		{
			name:       "nil task result",
			task:       &model.Task{},
			taskResult: nil,
		},
		{
			name: "missing billing context",
			task: &model.Task{},
			taskResult: &relaycommon.TaskInfo{
				TotalTokens: 1_000_000,
			},
		},
		{
			name: "zero total tokens",
			task: &model.Task{
				PrivateData: model.TaskPrivateData{
					BillingContext: &model.TaskBillingContext{
						OriginModelName: "doubao-seedance-2-0-filter-off",
						GroupRatio:      1,
					},
				},
			},
			taskResult: &relaycommon.TaskInfo{},
		},
		{
			name: "legacy model keeps generic completion billing",
			task: &model.Task{
				PrivateData: model.TaskPrivateData{
					BillingContext: &model.TaskBillingContext{
						OriginModelName: "doubao-seedance-2-0-260128",
						GroupRatio:      1,
						OtherRatios: map[string]float64{
							videoInputRatioKey: 28.0 / 46.0,
						},
					},
				},
			},
			taskResult: &relaycommon.TaskInfo{
				TotalTokens: 1_000_000,
			},
		},
		{
			name: "invalid group ratio",
			task: &model.Task{
				PrivateData: model.TaskPrivateData{
					BillingContext: &model.TaskBillingContext{
						OriginModelName: "doubao-seedance-2-0-filter-off",
					},
				},
			},
			taskResult: &relaycommon.TaskInfo{
				TotalTokens: 1_000_000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&TaskAdaptor{}).AdjustBillingOnComplete(tt.task, tt.taskResult)
			assert.Zero(t, got)
		})
	}
}
