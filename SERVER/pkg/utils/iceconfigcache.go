package utils

import (
	"time"

	"github.com/jellydator/ttlcache/v3"

	"__GITHUB_HUBLIVE__protocol/hublive"
)

const (
	iceConfigTTLMin = 5 * time.Minute
)

type IceConfigCache[T comparable] struct {
	c *ttlcache.Cache[T, *hublive.ICEConfig]
}

func NewIceConfigCache[T comparable](ttl time.Duration) *IceConfigCache[T] {
	cache := ttlcache.New(
		ttlcache.WithTTL[T, *hublive.ICEConfig](max(ttl, iceConfigTTLMin)),
		ttlcache.WithDisableTouchOnHit[T, *hublive.ICEConfig](),
	)
	go cache.Start()

	return &IceConfigCache[T]{cache}
}

func (icc *IceConfigCache[T]) Stop() {
	icc.c.Stop()
}

func (icc *IceConfigCache[T]) Put(key T, iceConfig *hublive.ICEConfig) {
	icc.c.Set(key, iceConfig, ttlcache.DefaultTTL)
}

func (icc *IceConfigCache[T]) Get(key T) *hublive.ICEConfig {
	if it := icc.c.Get(key); it != nil {
		return it.Value()
	}
	return &hublive.ICEConfig{}
}
