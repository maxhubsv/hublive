// Copyright 2024 HubLive, Inc.
//
// Transition aliases for store extraction (Phase 1).
// These allow existing code within service/ to continue using
// ObjectStore, LocalStore, RedisStore etc. without import changes.
// Remove in Phase 8 after all imports are updated.

package service

import (
	"github.com/maxhubsv/hublive-server/pkg/store"
	localstore "github.com/maxhubsv/hublive-server/pkg/store/local"
	redisstore "github.com/maxhubsv/hublive-server/pkg/store/redis"
)

// Store implementation aliases

// Deprecated: Use localstore.LocalStore from pkg/store/local
type LocalStore = localstore.LocalStore

// Deprecated: Use redisstore.RedisStore from pkg/store/redis
type RedisStore = redisstore.RedisStore

// Constructor aliases

// Deprecated: Use localstore.NewLocalStore from pkg/store/local
var NewLocalStore = localstore.NewLocalStore

// Deprecated: Use redisstore.NewRedisStore from pkg/store/redis
var NewRedisStore = redisstore.NewRedisStore

// Interface aliases — these keep service.ObjectStore etc. working
// while we migrate callers to use store.ObjectStore directly.

// Deprecated: Use store.ObjectStore from pkg/store
type ObjectStore = store.ObjectStore

// Deprecated: Use store.ServiceStore from pkg/store
type ServiceStore = store.ServiceStore

// Deprecated: Use store.OSSServiceStore from pkg/store
type OSSServiceStore = store.OSSServiceStore

// Deprecated: Use store.EgressStore from pkg/store
type EgressStore = store.EgressStore

// Deprecated: Use store.IngressStore from pkg/store
type IngressStore = store.IngressStore

// Deprecated: Use store.SIPStore from pkg/store
type SIPStore = store.SIPStore

// Deprecated: Use store.AgentStore from pkg/store
type AgentStore = store.AgentStore
