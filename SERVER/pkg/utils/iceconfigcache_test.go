package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"__GITHUB_HUBLIVE__protocol/hublive"
)

func TestIceConfigCache(t *testing.T) {
	cache := NewIceConfigCache[string](10 * time.Second)
	t.Cleanup(cache.Stop)

	cache.Put("test", &hublive.ICEConfig{})
	require.NotNil(t, cache)
}
