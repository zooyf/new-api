package doubao

import (
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVideoInputRatioSeedanceAliases(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		want       float64
	}{
		{
			name:       "seedance 2.0 480p 720p base",
			model:      "doubao-seedance-2-0-filter-off",
			resolution: "720p",
			want:       1,
		},
		{
			name:       "seedance 2.0 1080p",
			model:      "doubao-seedance-2-0-filter-off",
			resolution: "1080p",
			want:       7.7 / 7.0,
		},
		{
			name:       "seedance 2.0 4k",
			model:      "doubao-seedance-2-0-filter-off",
			resolution: "4K",
			want:       4.0 / 7.0,
		},
		{
			name:       "seedance 2.0 fast base",
			model:      "doubao-seedance-2-0-fast-filter-off",
			resolution: "720p",
			want:       1,
		},
		{
			name:       "seedance 2.0 mini base",
			model:      "dreamina-seedance-2-0-mini-filter-off",
			resolution: "720p",
			want:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetVideoInputRatio(tt.model, tt.resolution, false)
			require.True(t, ok)
			assert.InDelta(t, tt.want, got, 0.000001)
		})
	}
}

func TestGetVideoInputRatioOfficialSeedanceModelsUnchanged(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		resolution string
		hasVideo   bool
		want       float64
	}{
		{
			name:       "official seedance 2.0 base",
			model:      "doubao-seedance-2-0-260128",
			resolution: "720p",
			want:       1,
		},
		{
			name:       "official seedance 2.0 base with video input",
			model:      "doubao-seedance-2-0-260128",
			resolution: "720p",
			hasVideo:   true,
			want:       28.0 / 46.0,
		},
		{
			name:       "official seedance 2.0 1080p",
			model:      "doubao-seedance-2-0-260128",
			resolution: "1080p",
			want:       51.0 / 46.0,
		},
		{
			name:       "official seedance 2.0 1080p with video input",
			model:      "doubao-seedance-2-0-260128",
			resolution: "1080p",
			hasVideo:   true,
			want:       31.0 / 46.0,
		},
		{
			name:       "official seedance 2.0 4k",
			model:      "doubao-seedance-2-0-260128",
			resolution: "4K",
			want:       26.0 / 46.0,
		},
		{
			name:       "official seedance 2.0 4k with video input",
			model:      "doubao-seedance-2-0-260128",
			resolution: "4K",
			hasVideo:   true,
			want:       16.0 / 46.0,
		},
		{
			name:       "official seedance 2.0 fast base",
			model:      "doubao-seedance-2-0-fast-260128",
			resolution: "720p",
			want:       1,
		},
		{
			name:       "official seedance 2.0 fast with video input",
			model:      "doubao-seedance-2-0-fast-260128",
			resolution: "720p",
			hasVideo:   true,
			want:       22.0 / 37.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetVideoInputRatio(tt.model, tt.resolution, tt.hasVideo)
			require.True(t, ok)
			assert.InDelta(t, tt.want, got, 0.000001)
		})
	}
}

func TestSeedanceResolutionPrefersTopLevelField(t *testing.T) {
	req := common.TaskSubmitReq{
		Size:       "720p",
		Resolution: "1080p",
		Metadata: map[string]interface{}{
			"resolution": "4K",
		},
	}

	assert.Equal(t, "1080p", seedanceResolution(req))
}
