package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetChannelBindingPreservesTokenAndChannelAffinity(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AssetChannelBinding{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM asset_channel_bindings") })

	require.NoError(t, UpsertAssetChannelBinding("asset-1", "seedance-cn", 59, 101))
	require.NoError(t, UpsertAssetChannelBinding("asset-2", "seedance-cn", 59, 101))

	channelID, found, err := ResolveAssetChannelBinding([]string{"asset-1", "asset-1", "asset-2"}, 101)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 59, channelID)

	_, _, err = ResolveAssetChannelBinding([]string{"asset-1"}, 202)
	assert.ErrorContains(t, err, "another API token")

	err = UpsertAssetChannelBinding("asset-1", "seedance-cn", 60, 101)
	assert.ErrorContains(t, err, "already bound")
}

func TestAssetChannelBindingRejectsPartialAndMixedReferences(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AssetChannelBinding{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM asset_channel_bindings") })
	require.NoError(t, UpsertAssetChannelBinding("asset-a", "seedance-cn-a", 59, 101))
	require.NoError(t, UpsertAssetChannelBinding("asset-b", "seedance-cn-b", 60, 101))

	_, _, err := ResolveAssetChannelBinding([]string{"asset-a", "missing"}, 101)
	assert.ErrorContains(t, err, "no channel binding")

	_, _, err = ResolveAssetChannelBinding([]string{"asset-a", "asset-b"}, 101)
	assert.ErrorContains(t, err, "different upstream channels")
}
