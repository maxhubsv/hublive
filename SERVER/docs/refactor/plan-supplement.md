# HubLive Server — Domain Modules Refactor Plan — BẢN BỔ SUNG

> **Bản bổ sung cho plan gốc v1 — Giải quyết tất cả vấn đề đã phát hiện + thêm sections mới**
> **Version:** v2 (final)
> **Date:** 2026-03-29

---

## Mục lục Bổ sung

- [S1. FIX: Dependency Rule — domain → sfu Vi phạm](#s1-fix-dependency-rule--domain--sfu-vi-phạm)
- [S2. FIX: rtc/types/ Migration Plan](#s2-fix-rtctypes-migration-plan)
- [S3. FIX: Participant Split — Shared Mutable State Strategy](#s3-fix-participant-split--shared-mutable-state-strategy)
- [S4. FIX: RoomManager & RoomAllocator — Layer Assignment](#s4-fix-roommanager--roomallocator--layer-assignment)
- [S5. FIX: Phase 2/3 Internal Coupling Analysis](#s5-fix-phase-23-internal-coupling-analysis)
- [S6. FIX: "Zero Behavior Changes" — Correction](#s6-fix-zero-behavior-changes--correction)
- [S7. FIX: Rollback Strategy Per Phase](#s7-fix-rollback-strategy-per-phase)
- [S8. FIX: File Count Reconciliation](#s8-fix-file-count-reconciliation)
- [S9. FIX: signal.go Migration Clarification](#s9-fix-signalgo-migration-clarification)
- [S10. FIX: SubscriptionPolicy Hot Path Performance](#s10-fix-subscriptionpolicy-hot-path-performance)
- [S11. NEW: Test Strategy — New Tests Per Phase](#s11-new-test-strategy--new-tests-per-phase)
- [S12. NEW: Circular Import Detection Tooling](#s12-new-circular-import-detection-tooling)
- [S13. NEW: Wire DI Migration Detail](#s13-new-wire-di-migration-detail)
- [S14. NEW: Transition Aliases & Deprecation Schedule](#s14-new-transition-aliases--deprecation-schedule)
- [S15. NEW: Concurrency Safety Audit Checklist](#s15-new-concurrency-safety-audit-checklist)
- [S16. REVISED: Dependency Flow (Replaces Section 5)](#s16-revised-dependency-flow-replaces-section-5)
- [S17. REVISED: Migration Phase Updates](#s17-revised-migration-phase-updates)
- [S18. REVISED: Verification Checklist (Replaces Section 10)](#s18-revised-verification-checklist-replaces-section-10)
- [S19. REVISED: Risk Matrix (Replaces Section 9)](#s19-revised-risk-matrix-replaces-section-9)
- [S20. NEW: Decision Log](#s20-new-decision-log)
- [S21. NEW: External Consumer Audit](#s21-new-external-consumer-audit)
- [S22. NEW: Protobuf Types in Domain Interfaces — Decision](#s22-new-protobuf-types-in-domain-interfaces--decision)
- [S23. NEW: Error Handling Across Package Boundaries](#s23-new-error-handling-across-package-boundaries)
- [S24. NEW: Benchmark Baseline — Pre-Refactor](#s24-new-benchmark-baseline--pre-refactor)
- [S25. NEW: SessionHandler Interface Location](#s25-new-sessionhandler-interface-location)

---

## S1. FIX: Dependency Rule — domain → sfu Vi phạm

### Vấn đề

Section 5.1 định nghĩa:
```
Layer 1 (Domain):  domain/room, domain/participant, domain/track, domain/agent
Layer 2 (Infra):   store/, transport/, routing/, sfu/, telemetry/
Rule: Layer N chỉ import Layer 0..N-1, KHÔNG import Layer N+1
```

Section 5.2 vi phạm ngay:
```
domain/participant/participant.go
  ├── sfu/ (DownTrack, Receiver — media engine)  ← Layer 1 → Layer 2 ❌
```

### Phân tích root cause

`participant.go` sử dụng các SFU types sau:
- `sfu.DownTrack` — subscriber-side track wrapper
- `sfu.Receiver` — publisher-side RTP receiver
- `sfu.Buffer` — RTP buffer access
- `sfu.StreamAllocator` — bandwidth allocation
- `sfu.ForwardStats` — forwarding statistics

Đây không phải chỉ vài interface calls — participant **trực tiếp tạo, configure, và quản lý** SFU objects. Tách bằng interface sẽ tạo ra **~20 interface methods** chỉ để wrap SFU, không có giá trị abstraction thực.

### Quyết định: Reclassify SFU sang Layer 1

```
Layer 0 (Foundation):  utils, config, metric
Layer 1 (Core):        sfu/, domain/room, domain/participant, domain/track, domain/agent
Layer 2 (Infra):       store/, transport/, routing/, telemetry/
Layer 3 (API):         api/
Layer 4 (Bootstrap):   service/ (server.go, wire.go)
```

**Lý do:**
1. SFU là **media processing engine** — nó là core business logic, không phải infrastructure
2. SFU đã well-organized (13 sub-packages), không cần refactor
3. Tạo interface layer giữa domain và sfu sẽ thêm ~20 wrapper methods cho zero practical benefit
4. Không ai sẽ swap out SFU implementation — nó không phải extension point
5. HubLive IS an SFU — SFU code IS domain logic

**Constraint mới:** `domain/*` packages có thể import `sfu/` nhưng KHÔNG import `store/`, `transport/`, `routing/`, `api/`, `service/`.

**Updated rule:**
```
Layer N import Layer 0..N-1:
  sfu/           → utils, config (Layer 0) ✅
  domain/*       → sfu, utils, config (Layer 0-1) ✅
  store/         → domain interfaces, utils (Layer 0-1) ✅
  transport/     → domain interfaces, sfu, utils (Layer 0-1) ✅
  routing/       → domain interfaces, utils (Layer 0-1) ✅
  api/           → domain, store interfaces, routing interfaces (Layer 0-2) ✅
  service/       → everything (Layer 0-3) ✅
```

---

## S2. FIX: rtc/types/ Migration Plan

### Vấn đề

Plan gốc nói "dần dần migrate" mà không có plan cụ thể. `rtc/types/` là shared interface hub — nếu không xử lý đúng sẽ có dual source of truth.

### Inventory of rtc/types/

```
rtc/types/
├── interfaces.go       ← ~40 interface definitions, THE critical file
├── typesfakes/          ← counterfeiter-generated fakes for testing
└── protocol_version.go  ← Protocol version constants
```

**Key interfaces in rtc/types/interfaces.go:**
- `LocalParticipant` — used by room, transport, agent, routing
- `MediaTrack` — used by room, participant, subscription
- `Room` — used by participant, service
- `SubscribedTrack` — used by participant
- `Participant` — base interface
- `TransportManager` — used by participant
- `EgressLauncher` — used by room, participant
- Nhiều callback types (OnTrackPublished, etc.)

### Migration strategy: Phased extraction, NOT big bang

**Phase 4 (Track & Room extraction):**
```
Bước 1: Tạo domain/track/interfaces.go
  - Define: MediaTrack, SubscribedTrack interfaces
  - These are NEW definitions that domain/track exports

Bước 2: Tạo domain/room/interfaces.go
  - Define: Room, TrackResolver, SubscriptionPolicy interfaces
  - These are NEW definitions that domain/room exports

Bước 3: rtc/types/interfaces.go — thêm type aliases:
  // Transition aliases — DO NOT remove until Phase 8
  type MediaTrack = track.MediaTrack        // alias, NOT new type
  type Room = room.Room                      // alias, NOT new type

Bước 4: Existing code tiếp tục import rtc/types — aliases make it transparent
```

**Phase 5 (Participant extraction):**
```
Bước 1: Tạo domain/participant/interfaces.go
  - Define: LocalParticipant, Publisher, Subscriber interfaces

Bước 2: rtc/types/interfaces.go — thêm aliases:
  type LocalParticipant = participant.LocalParticipant

Bước 3: Interfaces CÒN LẠI trong rtc/types/:
  - TransportManager → giữ tại rtc/types/ (transport-specific)
  - EgressLauncher → giữ tại rtc/types/ (cross-cutting concern)
  - Callback types → giữ tại rtc/types/ (used everywhere)
  - Protocol version → giữ tại rtc/types/
```

**Phase 8 (Cleanup):**
```
Bước 1: Tìm tất cả files import rtc/types chỉ cho aliased types
Bước 2: Update imports sang domain/* packages
Bước 3: Remove aliases từ rtc/types/interfaces.go
Bước 4: rtc/types/ final state — chỉ còn:
  - TransportManager interface
  - EgressLauncher interface
  - Callback type definitions
  - Protocol version
  - typesfakes/ (regenerate for remaining interfaces)
```

### Circular import prevention

```
domain/room/interfaces.go DEFINES Room interface
  ↓ (Room interface references LocalParticipant)
domain/participant/interfaces.go DEFINES LocalParticipant interface
  ↓ (LocalParticipant interface references MediaTrack)
domain/track/interfaces.go DEFINES MediaTrack interface
  ↑ (MediaTrack does NOT reference Room or LocalParticipant — no cycle)
```

**Import direction (acyclic):**
```
domain/track ← domain/participant ← domain/room
(track knows nothing about participant or room)
(participant knows about track, not room)
(room knows about both)
```

Nếu Room interface cần reference LocalParticipant:
```go
// domain/room/interfaces.go
import "github.com/HubLive/hublive-server/pkg/domain/participant"

type Room interface {
    GetParticipants() []participant.LocalParticipant  // ✅ room → participant OK
}
```

Nếu LocalParticipant interface cần reference Room (circular!):
```go
// KHÔNG LÀM THẾ NÀY:
// domain/participant/interfaces.go
// import "domain/room"  ← CIRCULAR

// THAY VÀO ĐÓ — dùng callback hoặc interface ở participant package:
type ParticipantRoom interface {
    Name() HubLive.RoomName
    // minimal subset that participant needs from room
}
```

---

## S3. FIX: Participant Split — Shared Mutable State Strategy

### Vấn đề

participant.go 4700 dòng — publisher và subscriber **share mutable state**:
- Transport handle (cùng PeerConnection)
- Signal sender (cùng SignalConnection)
- Grants (cùng permission model)
- Telemetry service
- Mutex locks
- State machine (JOINING/ACTIVE/DISCONNECTED)
- Close() coordination

### Shared state inventory

| Field | Used by Publisher | Used by Subscriber | Used by Core | Access Pattern |
|-------|:-:|:-:|:-:|----------------|
| `id`, `identity` | Read | Read | Read/Write | Set once, read many |
| `state` | Read | Read | Read/Write | State machine transitions |
| `grants` | Read (CanPublish) | Read (CanSubscribe) | Read/Write | Updated via permission changes |
| `transportManager` | Write (publish tracks) | Write (subscribe tracks) | Write (close) | Shared mutable, needs sync |
| `params.SignalConn` | Write (send signals) | Write (send signals) | Write (send signals) | Shared mutable, needs sync |
| `lock` (sync.RWMutex) | Lock/Unlock | Lock/Unlock | Lock/Unlock | Shared |
| `telemetry` | Write (metrics) | Write (metrics) | Write (metrics) | Stateless service, safe to share |
| `params.Logger` | Write | Write | Write | Thread-safe |
| `dirty` (batched updates) | Set dirty flags | Set dirty flags | Flush | Shared mutable |
| `pendingTracksLock` | Lock | — | — | Publisher only |
| `subscribedTracks` | — | Read/Write | — | Subscriber only |
| `UpTrackManager` | Read/Write | — | — | Publisher only |
| `SubscriptionManager` | — | Read/Write | — | Subscriber only |

### Strategy: SharedContext struct + Composition

```go
// pkg/domain/participant/shared.go

// SharedContext holds state shared between publisher and subscriber.
// Both PublisherState and SubscriberState receive a pointer to this.
// SharedContext is OWNED by ParticipantImpl — never outlives it.
type SharedContext struct {
    // Identity (immutable after creation)
    id       HubLive.ParticipantID
    identity HubLive.ParticipantIdentity
    sid      HubLive.ParticipantID

    // State machine (synchronized via stateMu)
    stateMu sync.RWMutex
    state   HubLive.ParticipantInfo_State

    // Grants (synchronized via grantsMu)
    grantsMu sync.RWMutex
    grants   *auth.ClaimGrants

    // Shared services (thread-safe, no additional sync needed)
    transport TransportManager       // interface — wraps PeerConnection access
    signal    SignalSender            // interface — wraps SignalConnection
    telemetry TelemetryService        // interface — metrics emission
    logger    logger.Logger           // thread-safe

    // Batched update coordination
    dirtyMu sync.Mutex
    dirty   *DirtyFlags
}

// Thread-safe accessors
func (sc *SharedContext) ID() HubLive.ParticipantID { return sc.id }
func (sc *SharedContext) Identity() HubLive.ParticipantIdentity { return sc.identity }

func (sc *SharedContext) State() HubLive.ParticipantInfo_State {
    sc.stateMu.RLock()
    defer sc.stateMu.RUnlock()
    return sc.state
}

func (sc *SharedContext) SetState(s HubLive.ParticipantInfo_State) {
    sc.stateMu.Lock()
    defer sc.stateMu.Unlock()
    sc.state = s
}

func (sc *SharedContext) CanPublish() bool {
    sc.grantsMu.RLock()
    defer sc.grantsMu.RUnlock()
    return sc.grants != nil && sc.grants.Video.GetCanPublish()
}

func (sc *SharedContext) CanSubscribe() bool {
    sc.grantsMu.RLock()
    defer sc.grantsMu.RUnlock()
    return sc.grants != nil && sc.grants.Video.GetCanSubscribe()
}

func (sc *SharedContext) MarkDirty(flag DirtyFlag) {
    sc.dirtyMu.Lock()
    defer sc.dirtyMu.Unlock()
    sc.dirty.Set(flag)
}

// SignalSender — interface to decouple from concrete SignalConnection
type SignalSender interface {
    SendTrackPublished(track *HubLive.TrackInfo) error
    SendTrackUnpublished(trackID HubLive.TrackID) error
    SendSubscriptionResponse(trackID HubLive.TrackID, err error) error
    SendRefreshToken(token string) error
    // ... other signal methods
}
```

```go
// pkg/domain/participant/participant.go

type ParticipantImpl struct {
    shared *SharedContext   // shared between publisher + subscriber

    publisher  *PublisherState
    subscriber *SubscriberState

    // Close coordination (owned by ParticipantImpl only)
    closeOnce sync.Once
    closed    chan struct{}
}

func NewParticipant(params ParticipantParams) *ParticipantImpl {
    shared := &SharedContext{
        id:        params.ID,
        identity:  params.Identity,
        state:     HubLive.ParticipantInfo_JOINING,
        grants:    params.Grants,
        transport: params.TransportManager,
        signal:    params.SignalSender,
        telemetry: params.Telemetry,
        logger:    params.Logger,
        dirty:     NewDirtyFlags(),
    }

    p := &ParticipantImpl{
        shared:     shared,
        publisher:  NewPublisherState(shared, params),
        subscriber: NewSubscriberState(shared, params),
        closed:     make(chan struct{}),
    }
    return p
}
```

```go
// pkg/domain/participant/publisher.go

type PublisherState struct {
    shared *SharedContext  // readonly pointer — never reassigned

    // Publisher-only state (no external sync needed)
    upTrackManager     *UpTrackManager
    upDataTrackManager *UpDataTrackManager
    pendingTracksLock  sync.Mutex
    pendingTracks      []*pendingTrackInfo
    rtcpCh             chan []rtcp.Packet
}

func NewPublisherState(shared *SharedContext, params ParticipantParams) *PublisherState {
    return &PublisherState{
        shared:         shared,
        upTrackManager: NewUpTrackManager(params),
        rtcpCh:         make(chan []rtcp.Packet, 50),
    }
}

func (ps *PublisherState) AddTrack(req *HubLive.AddTrackRequest) (*HubLive.TrackInfo, error) {
    if !ps.shared.CanPublish() {
        return nil, ErrNoPublishPermission
    }
    // ... existing AddTrack logic, using ps.shared.transport and ps.shared.signal
}
```

```go
// pkg/domain/participant/subscriber.go

type SubscriberState struct {
    shared *SharedContext  // readonly pointer

    // Subscriber-only state
    subscriptionManager *SubscriptionManager
    subscribedTracks    map[HubLive.TrackID]*SubscribedTrackState
    subscribedTracksMu  sync.RWMutex
    subscriptionLimiter *SubscriptionLimiter
}

func (ss *SubscriberState) SubscribeToTrack(trackID HubLive.TrackID) error {
    if !ss.shared.CanSubscribe() {
        return ErrNoSubscribePermission
    }
    // ... existing subscribe logic
}
```

### Lock ordering rule (prevent deadlocks)

```
Lock acquisition order (always follow this order):
  1. SharedContext.stateMu
  2. SharedContext.grantsMu
  3. SharedContext.dirtyMu
  4. PublisherState.pendingTracksLock
  5. SubscriberState.subscribedTracksMu

NEVER acquire lock N while holding lock M where M > N.
```

### Close() coordination

```go
func (p *ParticipantImpl) Close(sendLeave bool, reason types.ParticipantCloseReason) error {
    var err error
    p.closeOnce.Do(func() {
        // 1. State transition (atomic)
        p.shared.SetState(HubLive.ParticipantInfo_DISCONNECTED)

        // 2. Stop publisher (stops accepting new tracks)
        p.publisher.Close()

        // 3. Stop subscriber (unsubscribes from all tracks)
        p.subscriber.Close()

        // 4. Close transport
        if sendLeave {
            _ = p.shared.signal.SendLeave(/* ... */)
        }
        err = p.shared.transport.Close()

        // 5. Signal completion
        close(p.closed)
    })
    return err
}
```

---

## S4. FIX: RoomManager & RoomAllocator — Layer Assignment

### Vấn đề

Plan gốc nói `roommanager.go` và `roomallocator.go` "Stays in service/" nhưng:
1. Không giải thích tại sao
2. Không define layer
3. api/room_service.go data flow không rõ

### Phân tích

**RoomManager** hiện tại:
- Orchestrate room lifecycle: create, get, delete, list
- Manage participant lifecycle: start session, remove participant
- Bridge giữa routing (find node) và domain (create room object)
- Uses: store, routing, domain/room, domain/participant, transport
- ~800 dòng

**RoomAllocator** hiện tại:
- Decide which node gets a new room
- Uses: routing/selector, store (check existing rooms)
- ~200 dòng

### Quyết định: Application Service Layer

```
Layer 0 (Foundation):  utils, config, metric
Layer 1 (Core):        sfu/, domain/*
Layer 2 (Infra):       store/, transport/, routing/, telemetry/
Layer 2.5 (App):       service/roommanager.go, service/roomallocator.go  ← STAYS
Layer 3 (API):         api/
Layer 4 (Bootstrap):   service/server.go, service/wire.go
```

**RoomManager = Application Service** — it orchestrates domain objects + infrastructure. This is a well-known pattern (DDD Application Service layer). Nó không phải domain logic (domain không biết về routing/store), và không phải API handler (nó không biết về HTTP/Twirp).

**Data flow (revised):**
```
api/room_service.go (Twirp handler)
  → validates request, extracts auth grants
  → calls service/roommanager.go (application service)
    → roommanager uses routing/ to find/allocate node
    → roommanager uses store/ to persist room state
    → roommanager uses domain/room to create Room object
    → roommanager uses domain/participant to create Participant
  ← returns protobuf response

api/room_service.go does NOT call domain/* directly for operations
  that need routing or storage coordination.
api/room_service.go CAN call domain/* directly for simple queries
  that don't need orchestration (e.g., pure validation).
```

### IOService Clarification

`service/ioservice.go` is the **IO bridge** between local RTC sessions and the routing layer. Specifically:
- Forwards signal messages from remote nodes to local participants (and vice versa)
- Handles cross-node participant communication in multi-node Redis-backed deployments
- Implements `routing.MessageSink` and `routing.MessageSource` interfaces
- Depends on: `routing/` (message router), `domain/participant` (local participant access)
- Does NOT depend on: `store/`, `transport/`, `api/`

IOService is an **application service** — it orchestrates message flow between infrastructure (routing) and domain (participant). It stays at Layer 2.5 alongside RoomManager.

**Updated service/ contents:**
```
service/
├── server.go          ← Bootstrap + HTTP listener (Layer 4)
├── wire.go            ← DI wiring (Layer 4)
├── wire_gen.go        ← Auto-generated (Layer 4)
├── turn.go            ← TURN server (Layer 4)
├── roommanager.go     ← Application service — room/participant lifecycle orchestration (Layer 2.5)
├── roomallocator.go   ← Application service — node selection for new rooms (Layer 2.5)
└── ioservice.go       ← Application service — cross-node message bridge via routing layer (Layer 2.5)
```

**Updated migration map for roommanager.go:**
```
service/roommanager.go → Stays in service/ (application service layer)
  - Inject domain/* interfaces via constructor
  - Replace direct rtc.NewRoom() → domain/room.NewRoom()
  - Replace direct rtc.NewParticipant() → domain/participant.NewParticipant()
  - Update imports

service/roomallocator.go → Stays in service/ (application service layer)
  - No changes needed (already uses routing/selector interface)

service/ioservice.go → Stays in service/ (IO bridge)
  - Update imports for new domain/* packages
```

---

## S5. FIX: Phase 2/3 Internal Coupling Analysis

### Vấn đề

Plan assumes Phase 2 (extract API) và Phase 3 (extract transport) are independent, nhưng files trong `service/` có internal coupling.

### Dependency map of service/ files being moved

```
service/rtcservice.go (→ transport/websocket)
  imports:
    ├── service/signal.go (signal relay logic)
    ├── service/roommanager.go (join participant)
    ├── service/auth.go (token validation)
    └── routing/ (message router)

service/signal.go (→ transport/websocket)
  imports:
    ├── service/roommanager.go (participant operations)
    └── routing/ (signal forwarding)

service/whipservice.go (→ transport/whip)
  imports:
    ├── service/roommanager.go (join participant)
    ├── service/auth.go (token validation)
    └── rtc/ (peer connection)

service/roomservice.go (→ api/room_service.go)
  imports:
    ├── service/roommanager.go (room operations)
    ├── service/roomallocator.go (node allocation)
    └── store/ interfaces

service/auth.go (→ api/middleware/)
  imports:
    └── config/ (API key config)
    # NO coupling to other service/ files ✅

service/egress.go (→ api/egress_service.go)
  imports:
    ├── store/ interfaces
    └── routing/ (launcher)
    # NO coupling to roommanager ✅
```

### Key finding

`auth.go` has **zero coupling** to other service/ files — can move independently.

`rtcservice.go` and `signal.go` are **tightly coupled** to each other AND to `roommanager.go`. They CANNOT be extracted without first establishing `roommanager.go` as a stable interface boundary.

`roomservice.go`, `egress.go`, `ingress.go`, `sip.go` — loosely coupled, can move independently.

### Revised Phase order

**Phase 2A: Extract API handlers (loosely coupled files only)**
1. Move `service/auth.go` → `api/middleware/auth.go` ← zero coupling
2. Move `service/basic_auth.go` → `api/middleware/basic_auth.go` ← zero coupling
3. Move `service/roomservice.go` → `api/room_service.go`
4. Move `service/egress.go` → `api/egress_service.go`
5. Move `service/ingress.go` → `api/ingress_service.go`
6. Move `service/sip.go` → `api/sip_service.go`
7. Move `service/agentservice.go` → `api/agent_service.go`
8. Move `service/clientconfiguration_service.go` → `api/client_config_service.go`

**Phase 2B: Stabilize RoomManager interface**
1. Define `RoomManager` interface in `service/interfaces.go`:
   ```go
   type RoomManager interface {
       StartSession(ctx context.Context, roomName HubLive.RoomName, ...) error
       RemoveParticipant(roomName HubLive.RoomName, identity HubLive.ParticipantIdentity) error
       GetRoom(ctx context.Context, roomName HubLive.RoomName) *HubLive.Room
       // ... existing methods
   }
   ```
2. `rtcservice.go` và `signal.go` depend on interface, not concrete struct

**Phase 3: Extract Transport Layer**
1. Move `service/rtcservice.go` → `transport/websocket/handler.go`
2. Move `service/signal.go` → `transport/websocket/signal.go`
3. Move `service/whipservice.go` → `transport/whip/handler.go`
4. These files import `RoomManager` interface — no circular dependency

**Rationale:** By stabilizing RoomManager interface BEFORE extracting transport, we ensure transport files can cleanly depend on an interface rather than a concrete struct in a package they're being moved away from.

---

## S6. FIX: "Zero Behavior Changes" — Correction

### Vấn đề

Plan tuyên bố "Zero behavior changes" nhưng có ít nhất 3 potential behavior changes.

### Acknowledged behavior changes

#### Change 1: Route mounting order (Phase 3)

```go
// Before: deterministic order (hardcoded)
mux.Handle("/rtc", rtcService)      // registered first
mux.Handle("/whip", whipService)    // registered second

// After: registry iteration order
for _, t := range transportRegistry.GetAll() {
    t.Register(mux)  // order depends on map iteration → UNDEFINED in Go
}
```

**Mitigation:** Use `[]Transport` (ordered slice) instead of `map[string]Transport` in registry, or use `http.ServeMux` which matches by longest prefix (order-independent for non-overlapping paths).

```go
// Fix: TransportRegistry uses ordered slice
type TransportRegistry struct {
    mu         sync.RWMutex
    transports []Transport  // ordered, not map
    byName     map[string]int // name → index for lookup
}
```

**Actual risk:** Low. `http.ServeMux` in Go 1.22+ matches by longest path prefix, so `/rtc` and `/whip` don't conflict regardless of registration order. But we fix it anyway to be rigorous.

#### Change 2: Package initialization order

Moving code between packages changes `init()` execution order. While `service/` currently may rely on implicit init ordering.

**Mitigation:** Audit all `init()` functions in moved files. HubLive uses very few `init()` — mostly metric registration. These are order-independent.

**Verification step (add to each phase):**
```bash
grep -rn "func init()" pkg/ | sort
# Verify no init() functions have order-dependent side effects
```

#### Change 3: Error messages containing package names

Some error messages include package path information (via `fmt.Errorf` or `errors.New` with package context). After move, error message strings change.

**Mitigation:** This is expected and acceptable. Client SDKs should not parse error message strings. But document it for log monitoring teams.

### Revised claim

~~Zero behavior changes.~~

**Corrected:** No intentional behavior changes. Three minor mechanical changes acknowledged (route registration order, init order, error message paths) — all mitigated. No API contract changes. No data format changes. No protocol changes.

---

## S7. FIX: Rollback Strategy Per Phase

### Rollback classification

| Phase | Rollback Type | Complexity | Point of No Return? |
|-------|--------------|------------|---------------------|
| 1 (Store) | Git revert PR | Simple | No |
| 2A (API handlers) | Git revert PR | Simple | No |
| 2B (RoomManager interface) | Git revert PR | Simple | No |
| 3 (Transport) | Git revert PR | Medium | No |
| 4 (Track & Room) | Git revert PR | Medium | No — aliases provide compat |
| 5 (Participant split) | Git revert PR | **Complex** | **Soft** — after Phase 6 builds on it |
| 6 (WebRTC transport) | Git revert PR | Complex | Soft — coupled with Phase 5 |
| 7 (Agent + extensions) | Git revert PR | Medium | No |
| 8 (Cleanup) | **Cannot revert** | N/A | **Yes — removes aliases** |

### Rollback procedures

**Phase 1-4: Simple revert**
```bash
# Each phase is 1 PR. Revert = revert PR.
git revert --no-commit <phase-N-merge-commit>
# Fix any conflicts (usually just import paths)
mage Build && mage Test
```

**Phase 5 (Participant split): Complex revert**

If runtime bug discovered after Phase 5 merge:
1. **Option A:** Fix forward (preferred if bug is locatable)
2. **Option B:** Revert Phase 5 PR
   - Must also revert Phase 6 if already merged (Phase 6 depends on split participant)
   - Restore `rtc/participant.go` monolith
   - Restore `rtc/participant_signal.go`
   - Update imports back to `rtc`

**Pre-Phase-5 checkpoint:** Tag `pre-participant-split` for quick rollback reference.

**Phase 8: Point of no return**

Phase 8 removes transition aliases. After Phase 8:
- External code using old import paths breaks
- Cannot revert without re-adding all aliases
- Must be confident ALL phases work correctly before Phase 8

**Rule:** Phase 8 only executes after **2 weeks** of production running Phase 1-7 code with zero issues.

### Canary deployment strategy (for Phase 5)

```
Day 1: Merge Phase 5 PR
Day 1-3: Deploy to staging, run full integration test suite
Day 3: Deploy to 1 canary node in production
Day 3-7: Monitor canary for:
  - Participant join/leave errors
  - Track publish/subscribe failures
  - Signal handling errors
  - Memory leaks (goroutine count)
  - Latency regression (P99 signal handling time)
Day 7: If clean → roll out to all nodes
Day 7: If issues → revert Phase 5 PR, investigate
```

---

## S8. FIX: File Count Reconciliation

### Vấn đề

Section 1 claims `service/` has 31 files, migration map lists ~22 entries.

### Full file inventory

```
pkg/service/ — COMPLETE inventory (31 files):

Source files (22):
  1.  server.go
  2.  wire.go
  3.  wire_gen.go
  4.  roomservice.go
  5.  egress.go
  6.  ingress.go
  7.  sip.go
  8.  agentservice.go
  9.  agent_dispatch_service.go
  10. rtcservice.go
  11. whipservice.go
  12. signal.go
  13. auth.go
  14. basic_auth.go
  15. interfaces.go
  16. localstore.go
  17. redisstore.go
  18. redisstore_sip.go
  19. roommanager.go
  20. roomallocator.go
  21. ioservice.go
  22. turn.go

Test files (7):
  23. roomservice_test.go
  24. egress_test.go
  25. localstore_test.go
  26. redisstore_test.go
  27. auth_test.go
  28. wire_test.go
  29. signal_test.go

Generated fakes (1 directory):
  30. servicefakes/ (counterfeiter-generated, contains ~2 files)

Config/misc (1):
  31. clientconfiguration_service.go  ← MISSED in migration map

Total: 31 ✅
```

### Missing from migration map — additions needed

| File | Destination | Phase |
|------|------------|-------|
| `clientconfiguration_service.go` | `api/client_config_service.go` | 2A |

### clientconfiguration_service.go Coupling Analysis

`clientconfiguration_service.go` handles client codec configuration requests (video codec preferences, simulcast settings). Analysis:
- Implements Twirp `ClientConfiguration` service interface
- Reads from `clientconfiguration/` package (pure config, no state)
- Does NOT depend on `roommanager.go`, `store/`, or `routing/`
- Stateless handler — no mutable state, no participant/room references

**Verdict:** Pure stateless Twirp handler. Zero coupling to other `service/` files. Safe to move in Phase 2A alongside other API handlers with no special considerations.
| `roomservice_test.go` | `api/room_service_test.go` | 2A |
| `egress_test.go` | `api/egress_service_test.go` | 2A |
| `auth_test.go` | `api/middleware/auth_test.go` | 2A |
| `localstore_test.go` | `store/local/store_test.go` | 1 |
| `redisstore_test.go` | `store/redis/store_test.go` | 1 |
| `signal_test.go` | `transport/websocket/signal_test.go` | 3 |
| `wire_test.go` | Stays in `service/` | — |
| `servicefakes/` | Regenerate per phase as interfaces move (see below) | 1-7 |

**Rule:** Test files ALWAYS move with their source files. This was implicit but must be explicit.

### servicefakes/ Regeneration Strategy

`servicefakes/` contains counterfeiter-generated fake implementations used in tests. These fakes are generated FROM interface definitions. When interfaces move between packages, fakes must be regenerated immediately — NOT deferred to Phase 8.

**Per-phase fake regeneration:**

| Phase | Interface Moved | Fake Action |
|-------|----------------|-------------|
| 1 | `ObjectStore` → `store.ServiceStore` | Generate `store/storefakes/` with `go generate ./pkg/store/...` |
| 2A | Service handler interfaces | Generate `api/apifakes/` if tests use fakes |
| 3 | `Transport` interface created | Generate `transport/transportfakes/` |
| 4 | `MediaTrack`, `Room` interfaces | Generate `domain/track/trackfakes/`, `domain/room/roomfakes/` |
| 5 | `LocalParticipant` interface | Generate `domain/participant/participantfakes/` |
| 7 | `AgentDispatcher`, `WorkerRegistry` | Generate `domain/agent/agentfakes/` |
| 8 | Remove old `servicefakes/` + `rtc/types/typesfakes/` entries for migrated interfaces |

**Procedure per phase:**
```bash
# 1. After moving interface to new package, add counterfeiter directive:
#    //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

# 2. Regenerate fakes:
go generate ./pkg/store/...
go generate ./pkg/domain/...

# 3. Update test imports to use new fake locations:
#    OLD: service/servicefakes.FakeObjectStore
#    NEW: store/storefakes.FakeServiceStore

# 4. Verify: go test ./... passes with new fakes
```

**Critical:** If tests in untouched packages (e.g., `routing/`) import `servicefakes.FakeObjectStore`, those imports must be updated when the fake moves. Grep for all `servicefakes` references per phase.

---

## S9. FIX: signal.go Migration Clarification

### Vấn đề

Migration map says `signal.go → transport/websocket/signal.go` nhưng section 4.5 doesn't describe it.

### signal.go analysis

`service/signal.go` contains:
- `SignalServer` struct — manages signal message relay between client and room
- `HandleSignal()` — entry point from WebSocket handler
- Signal message routing (dispatch incoming protobuf messages)
- Session lifecycle hooks

### Clarified destination

```
service/signal.go → transport/websocket/signal.go (Phase 3)

SignalServer struct becomes part of WebSocket transport:
  - WebSocketTransport creates SignalServer per connection
  - SignalServer delegates to RoomManager interface for participant operations
  - SignalServer delegates to domain/participant for signal message handling

Updated transport/websocket/ structure:
  transport/websocket/
  ├── handler.go     ← WebSocket upgrade, connection management (from rtcservice.go)
  ├── signal.go      ← SignalServer, message relay (from signal.go)
  └── signal_test.go ← (from signal_test.go)
```

### Section 4.5 addition

Add to section 4.5 after websocket/handler.go description:

```
**websocket/signal.go — từ service/signal.go:**
  Move logic từ service/signal.go:
    - SignalServer struct
    - HandleSignal() — signal message dispatch
    - Session lifecycle management
    - Giữ nguyên tất cả signal relay logic
    - Import RoomManager interface (not concrete struct)
```

---

## S10. FIX: SubscriptionPolicy Hot Path Performance

### Vấn đề

Plan claims "Extension interfaces chỉ được gọi 1 lần khi setup" — nhưng SubscriptionPolicy.ShouldSubscribe() và OnTrackPublished() are called in hot paths.

### Call frequency analysis

```
TransportRegistry.Register()     → called once at startup ✅ (not hot path)
RouterPlugin.SelectNode()        → called per room creation ✅ (not hot path)
AgentDispatcher.Dispatch()       → called per agent job ✅ (not hot path)

SubscriptionPolicy.ShouldSubscribe()   → called per (participant × track) ❌ HOT PATH
  Room with 100 participants, 100 tracks = 10,000 calls
  New participant joins = 100 calls (check each existing track)
  New track published = 100 calls (check each participant)

SubscriptionPolicy.OnTrackPublished()  → called per track publish ⚠️ WARM PATH
  Each track publish = 1 call, returns list of subscribers
```

### Performance mitigation

```go
// pkg/domain/room/interfaces.go

// SubscriptionPolicy determines which tracks a participant auto-subscribes to.
//
// PERFORMANCE NOTE: ShouldSubscribe() is called on the hot path —
// O(participants × tracks) per room join. Implementations MUST:
//   - Complete in < 1μs per call (no I/O, no locks, no allocations)
//   - Be safe for concurrent calls from multiple goroutines
//   - Cache decisions if computation is expensive
type SubscriptionPolicy interface {
    ShouldSubscribe(subscriber LocalParticipant, track MediaTrack) bool
    OnTrackPublished(room Room, track MediaTrack) []LocalParticipant
}
```

```go
// Default implementation — zero overhead (inlined by compiler)
type DefaultSubscriptionPolicy struct{}

func (d DefaultSubscriptionPolicy) ShouldSubscribe(
    subscriber LocalParticipant, track MediaTrack,
) bool {
    // Preserves current HubLive behavior: subscribe to all tracks
    // except own tracks and when subscription permission is denied
    return subscriber.CanSubscribe() &&
           track.PublisherID() != subscriber.ID()
}

func (d DefaultSubscriptionPolicy) OnTrackPublished(
    room Room, track MediaTrack,
) []LocalParticipant {
    // Return all participants that should subscribe
    result := make([]LocalParticipant, 0, len(room.GetParticipants()))
    for _, p := range room.GetParticipants() {
        if d.ShouldSubscribe(p, track) {
            result = append(result, p)
        }
    }
    return result
}
```

**Benchmark requirement (add to Phase 7):**
```go
// pkg/domain/room/auto_subscription_test.go

func BenchmarkDefaultSubscriptionPolicy_ShouldSubscribe(b *testing.B) {
    policy := DefaultSubscriptionPolicy{}
    subscriber := newMockParticipant()
    track := newMockTrack()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        policy.ShouldSubscribe(subscriber, track)
    }
    // Target: < 50ns/op, 0 allocs/op
}

func BenchmarkDefaultSubscriptionPolicy_OnTrackPublished_100Participants(b *testing.B) {
    policy := DefaultSubscriptionPolicy{}
    room := newMockRoomWithParticipants(100)
    track := newMockTrack()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        policy.OnTrackPublished(room, track)
    }
    // Target: < 5μs/op for 100 participants
}
```

**Updated claim:**

~~Extension interfaces chỉ được gọi 1 lần khi setup, không trong hot path.~~

**Corrected:** Transport, Router, and Agent extension interfaces are called only during setup/per-operation (not hot path). **SubscriptionPolicy is an exception** — `ShouldSubscribe()` is called in the hot path. Default implementation has zero overhead; custom implementations must meet <1μs/call requirement.

---

## S11. NEW: Test Strategy — New Tests Per Phase

### Principle

Mỗi phase cần 3 categories of tests:
1. **Migration tests** — verify moved code still works (existing tests, updated imports)
2. **Interface compliance tests** — verify new interfaces are correctly implemented
3. **Integration tests** — verify cross-package interactions

### Tests per phase

#### Phase 1 (Store)
```
New tests:
  store/local/store_test.go     ← moved from service/localstore_test.go
  store/redis/store_test.go     ← moved from service/redisstore_test.go
  store/interfaces_test.go      ← NEW: verify ServiceStore composite interface

New interface compliance test:
  func TestLocalStoreImplementsServiceStore(t *testing.T) {
      var _ store.ServiceStore = (*local.Store)(nil)  // compile-time check
  }

  func TestRedisStoreImplementsServiceStore(t *testing.T) {
      var _ store.ServiceStore = (*redis.Store)(nil)  // compile-time check
  }

New Redis compatibility test:
  func TestRedisKeyPatterns(t *testing.T) {
      // Verify all Redis key constants match expected patterns
      assert.Equal(t, "rooms", store.RoomsKey)
      assert.Equal(t, "room_locks", store.RoomLocksKey)
      // ... all keys
  }
```

#### Phase 2 (API)
```
Moved tests:
  api/room_service_test.go
  api/egress_service_test.go
  api/middleware/auth_test.go

New tests:
  api/middleware/auth_test.go    ← add test for middleware chain order
  api/room_service_test.go      ← add test verifying Twirp path constants unchanged
```

#### Phase 3 (Transport)
```
Moved tests:
  transport/websocket/signal_test.go

New tests:
  transport/registry_test.go    ← NEW:
    func TestTransportRegistry_Register(t *testing.T)
    func TestTransportRegistry_DuplicateProtocol(t *testing.T)
    func TestTransportRegistry_GetAll_Order(t *testing.T)

  transport/websocket/handler_test.go ← NEW:
    func TestWebSocketTransport_ImplementsInterface(t *testing.T) {
        var _ transport.Transport = (*websocket.WebSocketTransport)(nil)
    }

  transport/whip/handler_test.go ← NEW:
    func TestWHIPTransport_ImplementsInterface(t *testing.T) {
        var _ transport.Transport = (*whip.WHIPTransport)(nil)
    }
```

#### Phase 4 (Track & Room)
```
New interface compliance tests:
  domain/track/interfaces_test.go
  domain/room/interfaces_test.go

New tests:
  domain/room/auto_subscription_test.go ← NEW:
    func TestDefaultSubscriptionPolicy_SubscribeToAll(t *testing.T)
    func TestDefaultSubscriptionPolicy_SkipOwnTracks(t *testing.T)
    func TestDefaultSubscriptionPolicy_RespectPermissions(t *testing.T)
```

#### Phase 5 (Participant)
```
New tests:
  domain/participant/participant_test.go ← NEW:
    func TestParticipantStateMachine(t *testing.T)
    func TestParticipantClose_Coordination(t *testing.T)
    func TestSharedContext_ConcurrentAccess(t *testing.T)

  domain/participant/publisher_test.go ← NEW:
    func TestPublisher_AddTrack_NoPermission(t *testing.T)
    func TestPublisher_SharedContext_Integration(t *testing.T)

  domain/participant/subscriber_test.go ← NEW:
    func TestSubscriber_Subscribe_NoPermission(t *testing.T)
    func TestSubscriber_SharedContext_Integration(t *testing.T)

  domain/participant/signal_handler_test.go ← NEW:
    func TestSignalHandler_DispatchToPublisher(t *testing.T)
    func TestSignalHandler_DispatchToSubscriber(t *testing.T)

CRITICAL new test:
  domain/participant/integration_test.go ← NEW:
    func TestParticipant_PublishThenSubscribe(t *testing.T)
    // Full lifecycle: create → publish track → another participant subscribes
    // This catches bugs from incorrect state sharing between publisher/subscriber
```

#### Phase 6 (WebRTC Transport)
```
New tests:
  transport/webrtc/connection_test.go
  transport/webrtc/negotiation_test.go
  transport/webrtc/ice_test.go
```

#### Phase 7 (Agent + Extensions)
```
New tests:
  domain/agent/dispatch_test.go
  domain/agent/worker_test.go

Extension point verification tests:
  transport/registry_test.go ← TestCustomTransport_Registration
  domain/room/auto_subscription_test.go ← TestCustomSubscriptionPolicy_Injection
  routing/plugin_test.go ← TestNullRouterPlugin_NoOp
  store/interfaces_test.go ← TestCustomStore_Implementation

Performance benchmark:
  domain/room/auto_subscription_test.go ← BenchmarkDefaultSubscriptionPolicy
```

### Total new test count estimate

| Phase | Moved Tests | New Tests | New Benchmarks |
|-------|:-----------:|:---------:|:--------------:|
| 1 | 2 | 2 | 0 |
| 2 | 3 | 2 | 0 |
| 3 | 1 | 4 | 0 |
| 4 | 0 | 5 | 0 |
| 5 | 0 | 8 | 0 |
| 6 | 0 | 3 | 0 |
| 7 | 0 | 5 | 2 |
| **Total** | **6** | **29** | **2** |

---

## S12. NEW: Circular Import Detection Tooling

### Automated checks per phase

```bash
#!/bin/bash
# scripts/check-imports.sh — run after every phase

set -euo pipefail

echo "=== Checking for circular imports ==="
# Go compiler catches these, but explicit check gives better error messages
go build ./... 2>&1 | grep -i "import cycle" && {
    echo "FAIL: Circular import detected"
    exit 1
} || echo "PASS: No circular imports"

echo ""
echo "=== Checking dependency rule violations ==="

# Domain layer must NOT import infrastructure
VIOLATIONS=$(grep -rn '"github.com/HubLive/hublive-server/pkg/store' pkg/domain/ || true)
VIOLATIONS+=$(grep -rn '"github.com/HubLive/hublive-server/pkg/transport' pkg/domain/ || true)
VIOLATIONS+=$(grep -rn '"github.com/HubLive/hublive-server/pkg/routing' pkg/domain/ || true)
VIOLATIONS+=$(grep -rn '"github.com/HubLive/hublive-server/pkg/api' pkg/domain/ || true)
VIOLATIONS+=$(grep -rn '"github.com/HubLive/hublive-server/pkg/service' pkg/domain/ || true)

if [ -n "$VIOLATIONS" ]; then
    echo "FAIL: Domain layer importing infrastructure:"
    echo "$VIOLATIONS"
    exit 1
fi
echo "PASS: Domain layer imports clean"

echo ""
echo "=== Checking for orphan old-path imports ==="

# After each phase, verify no old paths remain
OLD_PATHS=(
    # Phase 1
    '"github.com/HubLive/hublive-server/pkg/service".LocalStore'
    '"github.com/HubLive/hublive-server/pkg/service".RedisStore'
    # Phase 2
    '"github.com/HubLive/hublive-server/pkg/service".RoomService'
    # Phase 3
    '"github.com/HubLive/hublive-server/pkg/service".RTCService'
    # Add more as phases complete
)

for path in "${OLD_PATHS[@]}"; do
    if grep -rn "$path" pkg/ --include="*.go" | grep -v "_test.go" | grep -v "compat.go"; then
        echo "WARNING: Old import path found: $path"
    fi
done

echo ""
echo "=== Layer dependency matrix ==="
# Visual dependency check
echo "domain/ imports:"
grep -roh '"github.com/HubLive/hublive-server/pkg/[^"]*"' pkg/domain/ | sort -u | sed 's/.*pkg\//  /'

echo "store/ imports:"
grep -roh '"github.com/HubLive/hublive-server/pkg/[^"]*"' pkg/store/ | sort -u | sed 's/.*pkg\//  /'

echo "transport/ imports:"
grep -roh '"github.com/HubLive/hublive-server/pkg/[^"]*"' pkg/transport/ | sort -u | sed 's/.*pkg\//  /'

echo "api/ imports:"
grep -roh '"github.com/HubLive/hublive-server/pkg/[^"]*"' pkg/api/ | sort -u | sed 's/.*pkg\//  /'
```

### CI integration

```yaml
# .github/workflows/refactor-checks.yml
# Add to existing CI pipeline during refactor period

- name: Check import rules
  run: bash scripts/check-imports.sh

- name: Check no init() ordering issues
  run: |
    # List all init() functions and verify independence
    grep -rn "func init()" pkg/ | sort
    # Manual review: each init() should be idempotent and order-independent
```

---

## S13. NEW: Wire DI Migration Detail

### Vấn đề

Plan mentions updating `wire.go` many times but doesn't show the actual provider set changes.

### Wire provider set evolution

#### Phase 1 — Store providers
```go
// pkg/service/wire.go changes

// BEFORE:
var ServiceSet = wire.NewSet(
    NewLocalStore,          // service.NewLocalStore
    NewRedisStore,          // service.NewRedisStore
    // ... other providers
)

// AFTER Phase 1:
import (
    localstore "github.com/HubLive/hublive-server/pkg/store/local"
    redisstore "github.com/HubLive/hublive-server/pkg/store/redis"
)

var ServiceSet = wire.NewSet(
    localstore.NewStore,     // store/local.NewStore
    redisstore.NewStore,     // store/redis.NewStore
    // bind interface
    wire.Bind(new(store.ServiceStore), new(*redisstore.Store)),
    // ... other providers
)
```

#### Phase 2 — API providers
```go
// AFTER Phase 2:
import (
    "github.com/HubLive/hublive-server/pkg/api"
    "github.com/HubLive/hublive-server/pkg/api/middleware"
)

var ServiceSet = wire.NewSet(
    // Store
    localstore.NewStore,
    redisstore.NewStore,
    // API handlers
    api.NewRoomService,
    api.NewEgressService,
    api.NewIngressService,
    api.NewSIPService,
    api.NewAgentService,
    // Middleware
    middleware.NewAuthMiddleware,
    // ... other providers
)
```

#### Phase 3 — Transport providers
```go
// AFTER Phase 3:
import (
    "github.com/HubLive/hublive-server/pkg/transport"
    "github.com/HubLive/hublive-server/pkg/transport/websocket"
    "github.com/HubLive/hublive-server/pkg/transport/whip"
)

var ServiceSet = wire.NewSet(
    // Store, API (same as before)
    // ...

    // Transport
    transport.NewTransportRegistry,
    websocket.NewWebSocketTransport,
    whip.NewWHIPTransport,

    // Wire bindings for transport registration
    wire.Struct(new(TransportRegistration), "*"),
)

// TransportRegistration is a Wire helper that registers all transports
type TransportRegistration struct {
    Registry  *transport.TransportRegistry
    WebSocket *websocket.WebSocketTransport
    WHIP      *whip.WHIPTransport
}

// provider function that Wire calls
func RegisterTransports(
    registry *transport.TransportRegistry,
    ws *websocket.WebSocketTransport,
    whip *whip.WHIPTransport,
) *transport.TransportRegistry {
    registry.Register(ws)
    registry.Register(whip)
    return registry
}
```

#### Phase 7 — Extension point providers
```go
// AFTER Phase 7:
var ServiceSet = wire.NewSet(
    // ... all previous providers ...

    // Extension points with defaults
    room.NewDefaultSubscriptionPolicy,
    wire.Bind(new(room.SubscriptionPolicy), new(*room.DefaultSubscriptionPolicy)),

    routing.NewNullRouterPlugin,
    wire.Bind(new(routing.RouterPlugin), new(*routing.NullRouterPlugin)),

    agent.NewDefaultDispatcher,
    wire.Bind(new(agent.AgentDispatcher), new(*agent.DefaultDispatcher)),
)
```

### Wire regeneration command per phase

```bash
# After each wire.go edit:
cd pkg/service
wire
# Or if using mage:
mage Generate
# Verify:
go build ./...
```

---

## S14. NEW: Transition Aliases & Deprecation Schedule

### Alias pattern

```go
// pkg/service/compat.go — TEMPORARY file, created Phase 1, deleted Phase 8

// Phase 1 aliases (store extraction)
package service

import (
    "github.com/HubLive/hublive-server/pkg/store"
    localstore "github.com/HubLive/hublive-server/pkg/store/local"
    redisstore "github.com/HubLive/hublive-server/pkg/store/redis"
)

// Deprecated: Use store.ServiceStore directly.
type ObjectStore = store.ServiceStore

// Deprecated: Use localstore.Store directly.
type LocalStore = localstore.Store

// Deprecated: Use redisstore.Store directly.
type RedisStore = redisstore.Store
```

```go
// pkg/rtc/compat.go — TEMPORARY file, created Phase 4, deleted Phase 8

package rtc

import (
    "github.com/HubLive/hublive-server/pkg/domain/room"
    "github.com/HubLive/hublive-server/pkg/domain/track"
)

// Phase 4 aliases
// Deprecated: Use room.Room directly.
type Room = room.RoomImpl

// Deprecated: Use track.MediaTrack directly.
type MediaTrack = track.MediaTrackImpl
```

### Deprecation schedule

| Phase | Aliases Created | Aliases Removed | Window |
|-------|----------------|-----------------|--------|
| 1 | `service.ObjectStore`, `service.LocalStore`, `service.RedisStore` | — | — |
| 2 | `service.RoomService`, `service.EgressService`, etc. | — | — |
| 3 | `service.RTCService`, `service.WHIPService` | — | — |
| 4 | `rtc.Room`, `rtc.MediaTrack`, `rtc.SubscribedTrack` | — | — |
| 5 | `rtc.ParticipantImpl` | — | — |
| 6 | `rtc.PCTransport` | — | — |
| 7 | — | — | — |
| 8 | — | **ALL aliases removed** | 2 weeks after Phase 7 |

### External consumers

If any external code imports HubLive's internal packages (unlikely but possible):
- Phase 8 PR description must list all removed aliases
- Add `// Deprecated` comments with `go vet` deprecation support starting Phase 1
- Consider `go doc` deprecation notices

---

## S15. NEW: Concurrency Safety Audit Checklist

### Why this matters

Moving code between packages can subtly change lock scoping. A mutex protecting fields in a single struct may no longer protect fields split across two structs.

### Audit per phase

#### Phase 5 (Participant split) — CRITICAL

```
Before split:
  ParticipantImpl.lock protects:
    - state field
    - grants field
    - publishedTracks map
    - subscribedTracks map
    - pendingTracks slice
    - dirty flags
    - metadata

After split:
  SharedContext.stateMu  → protects state
  SharedContext.grantsMu → protects grants
  SharedContext.dirtyMu  → protects dirty flags
  PublisherState.pendingTracksLock → protects pendingTracks
  SubscriberState.subscribedTracksMu → protects subscribedTracks

AUDIT QUESTIONS:
  □ Was original lock ever held while accessing BOTH publishedTracks AND subscribedTracks?
    If yes → need coordination mechanism between publisher/subscriber locks
  □ Was original lock ever held while accessing state AND pendingTracks?
    If yes → need to acquire stateMu THEN pendingTracksLock (follow lock order)
  □ Are there any defer lock.Unlock() patterns that span publisher+subscriber operations?
    If yes → must restructure to respect new lock boundaries
  □ Are there goroutines launched under lock that access split state?
    If yes → must verify which lock they need post-split
```

#### Phase 4 (Track & Room) — MEDIUM

```
AUDIT QUESTIONS:
  □ Room.lock protects both participant map AND track manager state?
    If yes → verify both are still under same lock after split
  □ RoomTrackManager has its own lock?
    If yes → verify lock ordering with Room.lock is preserved
```

#### Phase 6 (WebRTC Transport) — MEDIUM

```
AUDIT QUESTIONS:
  □ PCTransport.lock protects PeerConnection AND negotiation state?
    If yes → after split into connection.go + negotiation.go, same lock must protect both
    Strategy: negotiation.go receives pointer to connection's lock
  □ ICE candidate buffering has its own lock?
    If yes → verify lock ordering preserved after extraction to ice.go
```

### go race detector verification

```bash
# Run after EVERY phase:
go test -race ./pkg/domain/...
go test -race ./pkg/transport/...
go test -race ./pkg/store/...
go test -race ./pkg/api/...

# Phase 5 specific (extended race detection):
go test -race -count=10 ./pkg/domain/participant/...
# -count=10 reruns tests to increase chance of detecting races
```

---

## S16. REVISED: Dependency Flow (Replaces Section 5)

### Updated Layer Definition

```
Layer 0 (Foundation):
  utils/                    — shared utilities
  config/                   — YAML config loading
  metric/                   — internal metrics
  clientconfiguration/      — client codec config

Layer 1 (Core — business logic + media engine):
  sfu/                      — media processing engine (13 sub-packages)
  domain/room/              — room lifecycle, subscription policy
  domain/participant/       — participant state, publish, subscribe
  domain/track/             — track management
  domain/agent/             — agent dispatch logic

Layer 2 (Infrastructure — pluggable backends):
  store/                    — storage abstraction + implementations
  transport/                — protocol handlers + registry
  routing/                  — node routing + message passing
  telemetry/                — webhooks, analytics, prometheus

Layer 2.5 (Application Services — orchestration):
  service/roommanager.go    — room lifecycle orchestration
  service/roomallocator.go  — node allocation logic
  service/ioservice.go      — IO bridge

Layer 3 (API — thin handlers):
  api/                      — Twirp handlers
  api/middleware/            — auth, recovery

Layer 4 (Bootstrap — startup/DI):
  service/server.go         — HTTP listener, shutdown
  service/wire.go           — DI wiring
  service/turn.go           — TURN server
```

### Updated Dependency Rules

```
Layer 0 → nothing (only stdlib + external libs)
Layer 1 → Layer 0 + sfu (within Layer 1, acyclic: track ← participant ← room)
Layer 2 → Layer 0 + Layer 1 interfaces (NOT concrete types)
Layer 2.5 → Layer 0 + Layer 1 + Layer 2
Layer 3 → Layer 0 + Layer 1 + Layer 2 interfaces + Layer 2.5
Layer 4 → ALL layers (wires everything together)

EXCEPTION: domain/* imports sfu/ concrete types (justified in S1)
```

### Updated Dependency Graph

```
service/server.go (Layer 4 — bootstrap)
  ├── api/*                        (Layer 3)
  ├── transport/registry           (Layer 2)
  ├── store/*                      (Layer 2)
  ├── routing/*                    (Layer 2)
  ├── service/roommanager          (Layer 2.5)
  └── domain/*                     (Layer 1)

api/room_service.go (Layer 3)
  ├── service/roommanager          (Layer 2.5 — orchestration)
  ├── store/interfaces             (Layer 2 — via DI, for simple queries)
  └── routing/interfaces           (Layer 2 — via DI)

service/roommanager.go (Layer 2.5 — application service)
  ├── domain/room                  (Layer 1)
  ├── domain/participant           (Layer 1)
  ├── store/interfaces             (Layer 2)
  ├── routing/interfaces           (Layer 2)
  └── transport/interfaces         (Layer 2)

transport/websocket/handler.go (Layer 2)
  ├── service/roommanager interface (Layer 2.5 — via interface, not concrete)
  └── routing/interfaces           (Layer 2)

domain/room/room.go (Layer 1)
  ├── domain/participant/interfaces (Layer 1)
  ├── domain/track/interfaces      (Layer 1)
  └── sfu/                         (Layer 1)

domain/participant/participant.go (Layer 1)
  ├── domain/track/interfaces      (Layer 1)
  └── sfu/                         (Layer 1)

domain/track/media_track.go (Layer 1)
  └── sfu/                         (Layer 1)

store/redis/store.go (Layer 2)
  └── store/interfaces             (Layer 2)

store/local/store.go (Layer 2)
  └── store/interfaces             (Layer 2)
```

### Note on transport → roommanager dependency

`transport/websocket/handler.go` needs to call RoomManager (to join participants). This creates a Layer 2 → Layer 2.5 dependency. Solution:

```go
// pkg/service/interfaces.go (or a new shared interfaces location)

// SessionHandler is the interface transport uses to initiate sessions.
// Implemented by RoomManager.
// Defined at Layer 2 boundary so transport can import it.
type SessionHandler interface {
    StartSession(ctx context.Context, roomName HubLive.RoomName, ...) error
    // minimal interface — only what transport needs
}
```

Transport imports the interface (Layer 2). RoomManager implements it (Layer 2.5). Wire injects at Layer 4.

---

## S17. REVISED: Migration Phase Updates

### Updated phase list (replaces Section 6)

```
Phase 0:   Preparation                   (NEW — audit + baseline before any code changes)
Phase 1:   Extract Store Layer          (unchanged)
Phase 2A:  Extract API Handlers         (split from Phase 2)
Phase 2B:  Stabilize RoomManager Interface  (NEW — enables clean Phase 3)
Phase 3:   Extract Transport Layer      (unchanged scope, depends on 2B)
Phase 4:   Extract Domain — Track & Room (unchanged + rtc/types aliases)
Phase 5:   Split Participant God Object  (unchanged + SharedContext strategy)
Phase 6:   Split WebRTC Transport        (unchanged)
Phase 7:   Agent Domain + Extensions     (unchanged + benchmark requirement)
Phase 8:   Cleanup & Documentation       (unchanged + 2-week wait rule)
```

### Phase 0: Preparation (NEW — before any code changes)

**Scope:** Audit, baseline, and prerequisites

**Steps:**
1. Run external consumer audit (S21) — verify HubLive-cli/egress/ingress/agents imports
2. Run error audit (S23) — identify all sentinel errors and cross-package type assertions
3. Run `servicefakes/` usage audit — grep all `servicefakes` references across codebase
4. Create missing benchmarks (S24) — ensure BenchmarkRoom_*, BenchmarkParticipant_*, etc. exist
5. Run `benchmark-baseline.sh` — save results to `benchmarks/baseline/`
6. Tag commit as `pre-refactor-baseline`
7. Confirm Decision Log entries D1-D12 with team

**Files changed:** 0 production files. ~5-10 new test/benchmark files.
**Risk:** Zero — no production code changes.
**Output:** Baseline data + audit results that inform Phase 1-8 decisions.

**Verification:**
- [ ] External consumer audit complete — results documented
- [ ] Error audit complete — sentinel error inventory created
- [ ] Benchmark baseline saved and committed
- [ ] All decisions D1-D12 confirmed

### Phase 2B: Stabilize RoomManager Interface (NEW)

**Scope:** Define interface for RoomManager so transport can depend on abstraction

**Steps:**
1. Define `SessionHandler` interface in a shared location
2. Verify `roommanager.go` satisfies the interface
3. Update `rtcservice.go` and `signal.go` to use interface (still in `service/` at this point)
4. Run tests

**Files changed:** ~5
**Risk:** Low — interface extraction only

**Verification:**
- [ ] `mage Build` passes
- [ ] `mage Test` passes
- [ ] rtcservice.go no longer depends on concrete RoomManager struct

### Updated Phase 5 steps (additional)

Add after step 6:
```
6.5. Implement SharedContext struct (see S3)
     - Create shared.go with SharedContext, SignalSender interface
     - Define lock ordering rules (document in shared.go comments)
     - Publisher and Subscriber receive *SharedContext via constructor
6.6. Run race detector:
     go test -race -count=10 ./pkg/domain/participant/...
6.7. Run concurrency audit (see S15)
```

### Updated Phase 7 steps (additional)

Add after step 8:
```
8.5. Add benchmark tests for SubscriptionPolicy (see S10)
     go test -bench=. ./pkg/domain/room/...
     Verify: DefaultSubscriptionPolicy < 50ns/op, 0 allocs/op
```

---

## S18. REVISED: Verification Checklist (Replaces Section 10)

### Per Phase Checklist

```
BUILD:
  [ ] go build ./... passes (zero errors)
  [ ] go vet ./... passes
  [ ] mage Generate succeeds (Wire codegen)
  [ ] No unused imports (goimports -l)

TESTS:
  [ ] mage Test passes (unit tests)
  [ ] go test -race ./... passes (race detector)
  [ ] New interface compliance tests pass
  [ ] New unit tests written and passing

IMPORTS:
  [ ] scripts/check-imports.sh passes (see S12)
  [ ] No references to old package paths (grep verify)
  [ ] No circular imports
  [ ] Dependency layer rules respected

CONCURRENCY (Phase 4-6 only):
  [ ] Lock audit completed (see S15)
  [ ] go test -race -count=10 for affected packages
  [ ] Lock ordering documented in code comments

ERRORS (all phases):
  [ ] Sentinel errors moved with source files
  [ ] Error aliases added in old locations if cross-package refs exist (see S23)
  [ ] errors.Is() / errors.As() checks verified in tests

FAKES (all phases):
  [ ] counterfeiter fakes regenerated for moved interfaces
  [ ] Test imports updated to new fake locations
  [ ] grep for old servicefakes/typesfakes references — zero hits

PERFORMANCE (Phase 4+ only):
  [ ] Run phase benchmark suite, compare with baseline (see S24)
  [ ] No regression > 5% vs baseline
  [ ] SFU benchmarks unchanged (control group)
```

### After All Phases Checklist

```
FUNCTIONAL:
  [ ] mage TestAll passes (integration tests)
  [ ] mage Build produces working binary
  [ ] Server starts and accepts WebSocket connections
  [ ] Room create/delete via Twirp API works
  [ ] Participant join/leave via WebSocket works
  [ ] Track publish/subscribe works (audio + video)
  [ ] Data channel messaging works
  [ ] WHIP endpoint works
  [ ] Agent dispatch works
  [ ] Redis-backed multi-node mode works
  [ ] TURN relay works
  [ ] Prometheus metrics exposed

COMPATIBILITY:
  [ ] Redis key patterns unchanged (automated test)
  [ ] Twirp API paths unchanged (automated test)
  [ ] Protobuf message formats unchanged (no proto changes)
  [ ] Client SDK connection flow unchanged (manual test with JS SDK)

EXTENSION POINTS:
  [ ] Custom Transport registers and mounts (test)
  [ ] Custom SubscriptionPolicy can be injected (test)
  [ ] Custom Store implementation can be injected (test)
  [ ] Custom RouterPlugin can be injected (test)
  [ ] Custom AgentDispatcher can be injected (test)

PERFORMANCE:
  [ ] DefaultSubscriptionPolicy < 50ns/op, 0 allocs/op
  [ ] No goroutine leaks (before/after goroutine count)
  [ ] No measurable latency regression in signal handling (P99)
  [ ] Full benchmark suite vs Phase 0 baseline — no regression > 5% (benchstat)
  [ ] SFU benchmarks identical to baseline (control group — zero change expected)

ERRORS:
  [ ] All sentinel errors in final locations (no aliases remaining)
  [ ] errors.Is() / errors.As() checks pass for all moved error types
  [ ] No package-qualified error type assertions reference old paths

FAKES:
  [ ] All counterfeiter fakes regenerated in new package locations
  [ ] Old servicefakes/ only contains fakes for interfaces still in service/
  [ ] Old typesfakes/ only contains fakes for interfaces still in rtc/types/
  [ ] Zero orphan fake references (grep servicefakes + typesfakes across codebase)

EXTERNAL COMPATIBILITY (if external consumers found in S21):
  [ ] External repos build against new package structure
  [ ] Transition aliases maintained if required
  [ ] Migration guide published for external consumers

CODE QUALITY:
  [ ] go vet ./... clean
  [ ] All init() functions audited for order independence
  [ ] All transition aliases documented with Deprecated comments
  [ ] README.md updated with new structure
  [ ] compat.go files listed for Phase 8 removal
  [ ] Decision log D1-D12 committed to repo
```

### Size Metrics (updated)

| Metric | Before | After |
|--------|--------|-------|
| `service/` files (source) | 22 | 7 (server, wire, wire_gen, turn, roommanager, roomallocator, ioservice) |
| `service/` files (total w/ tests) | 31 | ~10 |
| `rtc/` top-level source files | 29 | 6-8 |
| Largest file | 4700 lines (participant.go) | ~1500 lines (connection.go) |
| Packages with >20 files | 2 (service, rtc) | 0 |
| Extension point interfaces | 0 | 5 |
| Domain packages | 0 | 4 (room, participant, track, agent) |
| New test files | — | ~15 |
| New benchmark files | — | ~8 |
| Transition alias files | — | 2 (removed in Phase 8) |
| Architectural decisions documented | 0 | 12 |
| Risks identified + mitigated | 0 | 17 |
| Migration phases (including Phase 0) | — | 10 |

---

## S19. REVISED: Risk Matrix (Replaces Section 9)

| # | Risk | Probability | Impact | Mitigation | Detection |
|---|------|:-----------:|:------:|------------|-----------|
| 1 | Circular import after move | High | Build fail | Interface-based deps, DI injection, check-imports.sh script | `go build` fails immediately |
| 2 | God object split breaks behavior | Medium | Runtime bug | SharedContext pattern (S3), integration tests, -race flag, canary deploy | Integration tests, canary monitoring |
| 3 | Wire codegen fail | Medium | Build fail | Run `mage Generate` per phase, Wire errors are clear | `wire` command fails |
| 4 | Test imports broken | High | Test fail | Move tests with source files, grep for old paths | `go test` fails |
| 5 | Shared mutable state race after split | Medium | Data corruption | SharedContext (S3), lock ordering rules, -race -count=10 | Race detector, -race flag |
| 6 | Lock ordering deadlock after split | Low | Hang/deadlock | Document lock order in S3, audit per S15 | Timeout in tests, goroutine dump |
| 7 | Performance regression (SubscriptionPolicy) | Low | Latency | Benchmark requirement (S10), interface inlining by compiler | Benchmark tests |
| 8 | Redis compat broken | Low | Data loss | Key constant tests, grep verification | Automated Redis key test |
| 9 | Route mounting order change | Low | Wrong handler | Use ordered slice in registry (S6), Go 1.22 longest-prefix matching | Integration test for each endpoint |
| 10 | init() order change | Low | Startup bug | Audit all init() functions, verify idempotence | Manual audit + startup test |
| 11 | Merge conflicts with upstream | Medium | Time cost | Pin to tag, merge after refactor completes | Git merge |
| 12 | rtc/types dual source of truth | Medium | Confusion | Phased alias strategy (S2), clear deprecation schedule (S14) | Grep for duplicate interfaces |
| 13 | RoomManager layer confusion | Medium | Wrong deps | Explicitly define as Layer 2.5, interface for transport (S4) | check-imports.sh |
| 14 | compat.go aliases leak into long-term code | Low | Tech debt | Phase 8 hard deadline, Deprecated annotations, CI warning | Grep for compat.go imports |
| 15 | External repos import moved packages | Medium | Breaking change | Pre-Phase-1 audit (S21), maintain aliases if found | Audit script |
| 16 | Sentinel error type assertions break | Low | Runtime bug | Error audit (S23), move errors with source files, add aliases | grep + test |
| 17 | Performance regression undetected | Low | Latency | Benchmark baseline (S24), per-phase comparison with benchstat | benchstat comparison |

---

## S20. NEW: Decision Log

Record key architectural decisions made during planning. Reference these when questions arise during implementation.

| # | Decision | Alternatives Considered | Rationale |
|---|----------|------------------------|-----------|
| D1 | SFU classified as Layer 1 (Core), not Layer 2 (Infra) | (a) Create interface layer between domain and SFU, (b) Keep SFU at Layer 2 with exception | SFU IS core business logic. Interface layer would add ~20 wrapper methods for zero practical benefit. Nobody will swap SFU implementation. |
| D2 | RoomManager stays in `service/` as Application Service | (a) Move to domain/, (b) Move to new `app/` package | RoomManager orchestrates across domain + infra. It's not pure domain logic. Creating new `app/` package adds complexity for 3 files. |
| D3 | SharedContext pattern for participant split | (a) Back-pointer to ParticipantImpl, (b) Copy state, (c) Single giant interface | SharedContext is explicit about shared state, enables clear lock ordering, doesn't re-couple through back-pointers. |
| D4 | Type aliases for transition (not wrapper types) | (a) No aliases (big bang migration), (b) Wrapper types, (c) Intermediate interface package | Aliases are transparent to callers (no code changes needed), removed cleanly in Phase 8. Wrapper types add indirection. |
| D5 | Ordered slice (not map) for TransportRegistry | (a) map[string]Transport, (b) sync.Map | Map iteration order is undefined in Go. Ordered slice gives deterministic registration order. |
| D6 | Phase 2 split into 2A + 2B | (a) Single Phase 2 with all moves, (b) Three sub-phases | RoomManager interface must be stable before transport extraction. 2A (API) and 2B (interface) are clean separation points. |
| D7 | SubscriptionPolicy allows hot-path calls with benchmark requirement | (a) Only setup-time interfaces, (b) Pre-compute subscription table | Hot-path interface enables powerful extensions (bandwidth-aware, role-based). Benchmark requirement ensures no regression. Default impl inlines to near-zero cost. |
| D8 | rtc/types/ migrated via aliases, not big-bang | (a) Move all interfaces at once, (b) Keep everything in rtc/types forever | Phased approach prevents dual source of truth while allowing gradual migration. Hard deadline in Phase 8 prevents permanent tech debt. |
| D9 | 2-week production soak before Phase 8 | (a) Immediate cleanup, (b) 1-week soak, (c) 1-month soak | 2 weeks balances risk (catch runtime bugs) with velocity (don't delay too long). Phase 8 is point of no return. |
| D10 | transport/ depends on SessionHandler interface, not concrete RoomManager | (a) Direct dependency on RoomManager, (b) Event bus | Interface keeps Layer 2 → Layer 2.5 clean. Event bus adds complexity and debugging difficulty for one dependency. |
| D11 | Domain interfaces use protobuf types directly | (a) Create domain-specific value objects, (b) Use protobuf for API only + map at boundary | HubLive protobuf types ARE the domain model. Mapping layer adds ~30 functions for zero benefit. Constraint: no gRPC/Twirp types in domain. See S22. |
| D12 | SessionHandler interface defined in `transport/interfaces.go` | (a) Define in `service/`, (b) Define in new `shared/` package, (c) Define in `domain/` | Consumer-side interface definition is idiomatic Go. transport/ is Layer 2; service/ (Layer 2.5) can import it without cycle. See S25. |

---

## S21. NEW: External Consumer Audit

### Vấn đề

Plan assumes all consumers of `pkg/service/` and `pkg/rtc/` are internal to hublive-server. If external repos (HubLive-cli, HubLive-egress, HubLive-ingress, HubLive-agents) import these packages, transition aliases become critical for backward compatibility.

### Audit procedure (pre-Phase 1)

```bash
# Clone related repos and check imports:
REPOS=(
    "github.com/hublive/hublive-cli"
    "github.com/HubLive/egress"
    "github.com/HubLive/ingress"
    "github.com/HubLive/agents"
    "github.com/HubLive/server-sdk-go"
    "github.com/HubLive/sip"
)

for repo in "${REPOS[@]}"; do
    echo "=== $repo ==="
    # Check for imports of packages being moved
    grep -rn '"github.com/HubLive/hublive-server/pkg/service"' . 2>/dev/null || echo "  No service/ imports"
    grep -rn '"github.com/HubLive/hublive-server/pkg/rtc"' . 2>/dev/null || echo "  No rtc/ imports"
    grep -rn '"github.com/HubLive/hublive-server/pkg/rtc/types"' . 2>/dev/null || echo "  No rtc/types/ imports"
done
```

### Impact classification

| External Import Pattern | Impact | Mitigation |
|------------------------|--------|------------|
| No external imports found | None | Transition aliases are convenience only, can be aggressive about removal |
| `pkg/rtc/types` imported (likely) | High | rtc/types aliases MUST be maintained through Phase 8; consider permanent aliases |
| `pkg/service` types imported | Medium | compat.go aliases required; coordinate with consuming repo maintainers |
| `pkg/rtc` concrete types imported | High | Must provide migration guide for external consumers |

### Decision needed (add to D-Log)

If external imports exist → Phase 8 timeline extends. Aliases become permanent exports, not temporary transition aids. This changes the nature of Phase 8 from "cleanup" to "publish stable API."

If no external imports → proceed as planned. Aliases removed in Phase 8.

**This audit MUST complete before Phase 1 begins.**

---

## S22. NEW: Protobuf Types in Domain Interfaces — Decision

### Vấn đề

Domain layer interfaces use protobuf generated types in signatures:

```go
// domain/participant/interfaces.go
type Publisher interface {
    AddTrack(req *HubLive.AddTrackRequest) (*HubLive.TrackInfo, error)
    //          ^^^^^^^^^^^^^^^^^^^^^^^^     ^^^^^^^^^^^^^^^^^
    //          protobuf generated type       protobuf generated type
}
```

This is a design decision: should domain interfaces use protobuf types directly, or wrap them in domain-specific types?

### Decision: Use protobuf types directly (D11)

**Rationale:**
1. HubLive's protobuf types ARE the domain model — `HubLive.Room`, `HubLive.ParticipantInfo`, `HubLive.TrackInfo` are not wire-only types; they carry domain semantics
2. Creating domain wrapper types would add ~30 mapping functions (protobuf → domain → protobuf) for zero benefit
3. Protobuf types are already used everywhere in the codebase — introducing a parallel type system creates confusion
4. The only benefit of domain types would be if we planned to change the protobuf schema — but the plan explicitly forbids this
5. Go protobuf types are plain structs with no framework coupling — they're already "domain types" in practice

**Constraint:**
- Domain interfaces MAY use `HubLive.*` protobuf types in method signatures
- Domain interfaces MUST NOT import `google.golang.org/grpc` or Twirp-specific types — those belong in `api/` layer
- Domain structs MUST NOT embed protobuf message types as primary state — use protobuf for API boundaries, struct fields for internal state

**Example — correct usage:**
```go
// domain/participant/interfaces.go — OK: protobuf as API type
type Publisher interface {
    AddTrack(req *HubLive.AddTrackRequest) (*HubLive.TrackInfo, error)
}

// domain/participant/participant.go — OK: internal state uses plain fields
type ParticipantImpl struct {
    id       HubLive.ParticipantID    // type alias, not protobuf message
    identity HubLive.ParticipantIdentity // type alias
    state    HubLive.ParticipantInfo_State // protobuf enum — acceptable
    // NOT: info *HubLive.ParticipantInfo  ← don't embed full protobuf as state
}
```

### Add to Decision Log

| # | Decision | Alternatives Considered | Rationale |
|---|----------|------------------------|-----------|
| D11 | Domain interfaces use protobuf types directly | (a) Create domain-specific value objects, (b) Use protobuf for API only + map at boundary | HubLive protobuf types ARE the domain model. Mapping layer adds ~30 functions for zero benefit. Constraint: no gRPC/Twirp types in domain. |

---

## S23. NEW: Error Handling Across Package Boundaries

### Vấn đề

When files move between packages, error types and sentinel errors change their package path. Code using `errors.Is()` or `errors.As()` continues to work (they check underlying type/value). But code using package-qualified type assertions may break.

### Audit procedure

```bash
# Find all sentinel errors in packages being moved:
grep -rn 'var Err.*= errors.New\|var Err.*= fmt.Errorf' \
    pkg/service/ pkg/rtc/ pkg/agent/ \
    | grep -v _test.go | sort

# Find all custom error types:
grep -rn 'type.*Error struct' pkg/service/ pkg/rtc/ pkg/agent/ | sort

# Find all error type assertions against these packages:
grep -rn 'service\.Err\|rtc\.Err\|agent\.Err' pkg/ | grep -v _test.go | sort
grep -rn 'errors\.Is.*service\.\|errors\.As.*service\.' pkg/ | sort
```

### Known error patterns in HubLive

```
pkg/rtc/:
  - ErrPermission          → moves to domain/participant/errors.go
  - ErrTrackNotFound       → moves to domain/track/errors.go
  - ErrRoomClosed          → moves to domain/room/errors.go
  - ErrTransportClosed     → moves to transport/errors.go

pkg/service/:
  - ErrRoomNotFound        → moves to store/errors.go (or domain/room/errors.go)
  - ErrParticipantNotFound → moves to store/errors.go (or domain/participant/errors.go)
  - ErrOperationFailed     → moves to api/errors.go (if API-specific)
```

### Strategy: Centralized error package (optional) vs distributed

**Option A: Distributed (recommended)** — each domain package defines its own errors:
```go
// domain/room/errors.go
package room

var (
    ErrRoomNotFound = errors.New("room not found")
    ErrRoomClosed   = errors.New("room is closed")
)

// domain/participant/errors.go
package participant

var ErrPermission = errors.New("participant does not have permission")
```

**Option B: Centralized** — `pkg/errors/` package with all domain errors.
Rejected: creates a magnet package that everything imports, defeating modularization.

### Migration procedure per phase

```
1. Identify sentinel errors in files being moved
2. Move errors with their files (keep in same package)
3. If errors are referenced from OTHER packages:
   a. Add alias in old location: var ErrRoomNotFound = room.ErrRoomNotFound
   b. Update references in moved phase
   c. Remove alias in Phase 8
4. Verify: grep for old error references, ensure all updated
5. Test: errors.Is() checks still pass
```

### Add to per-phase checklist

```
ERRORS:
  [ ] All sentinel errors identified in moved files
  [ ] Error aliases added in old locations if cross-package references exist
  [ ] errors.Is() / errors.As() checks verified in tests
  [ ] No package-qualified error type assertions broken
```

---

## S24. NEW: Benchmark Baseline — Pre-Refactor

### Vấn đề

S10 adds benchmark requirements for SubscriptionPolicy post-refactor, but without a baseline, we can't prove "no regression." Need benchmarks BEFORE Phase 1 starts.

### Pre-refactor benchmark suite

Run these benchmarks on the CURRENT codebase before any refactoring begins. Save results as the baseline.

```bash
# Create baseline benchmark script
# Run on the unmodified codebase, save results

#!/bin/bash
# scripts/benchmark-baseline.sh
set -euo pipefail

BASELINE_DIR="benchmarks/baseline-$(date +%Y%m%d)"
mkdir -p "$BASELINE_DIR"

echo "=== Running baseline benchmarks ==="

# Room operations
go test -bench=BenchmarkRoom -benchmem -count=5 \
    ./pkg/rtc/... \
    | tee "$BASELINE_DIR/room.txt"

# Participant operations
go test -bench=BenchmarkParticipant -benchmem -count=5 \
    ./pkg/rtc/... \
    | tee "$BASELINE_DIR/participant.txt"

# Track operations
go test -bench=BenchmarkTrack -benchmem -count=5 \
    ./pkg/rtc/... \
    | tee "$BASELINE_DIR/track.txt"

# Store operations
go test -bench=BenchmarkStore -benchmem -count=5 \
    ./pkg/service/... \
    | tee "$BASELINE_DIR/store.txt"

# Signal handling
go test -bench=BenchmarkSignal -benchmem -count=5 \
    ./pkg/service/... \
    | tee "$BASELINE_DIR/signal.txt"

# SFU (should NOT change — control group)
go test -bench=. -benchmem -count=5 \
    ./pkg/sfu/... \
    | tee "$BASELINE_DIR/sfu.txt"

echo "=== Baseline saved to $BASELINE_DIR ==="
echo "Compare after each phase with:"
echo "  benchstat $BASELINE_DIR/room.txt benchmarks/phase-N/room.txt"
```

### Per-phase comparison

```bash
# After each phase:
PHASE_DIR="benchmarks/phase-N"
mkdir -p "$PHASE_DIR"

# Run same benchmarks
go test -bench=BenchmarkRoom -benchmem -count=5 ./pkg/domain/room/... \
    | tee "$PHASE_DIR/room.txt"

# Compare with baseline using benchstat
go install golang.org/x/perf/cmd/benchstat@latest
benchstat "$BASELINE_DIR/room.txt" "$PHASE_DIR/room.txt"

# Acceptance criteria:
# - No benchmark should regress by > 5% (within noise margin)
# - Zero new allocations in hot paths
# - SFU benchmarks should be IDENTICAL (control group)
```

### Key benchmarks to ensure exist BEFORE refactoring

If these benchmarks don't exist in the current codebase, create them before Phase 1:

```
Required benchmarks (create if missing):
  BenchmarkRoom_Join              — participant joining a room
  BenchmarkRoom_TrackPublish      — publishing a track to room
  BenchmarkRoom_AutoSubscribe     — auto-subscription on track publish (THIS becomes SubscriptionPolicy)
  BenchmarkParticipant_Create     — participant construction
  BenchmarkParticipant_AddTrack   — track addition
  BenchmarkSignal_Dispatch        — signal message routing
  BenchmarkStore_LoadRoom         — room loading (local store)
  BenchmarkStore_ListParticipants — participant listing (local store)
```

### Add to Phase 0 (pre-refactor step)

```
Phase 0: Preparation (before any code changes)
  1. Run external consumer audit (S21)
  2. Run error audit (S23)
  3. Create missing benchmarks listed above
  4. Run benchmark-baseline.sh, commit results to repo
  5. Tag commit as `pre-refactor-baseline`
```

---

## S25. NEW: SessionHandler Interface Location

### Vấn đề

S16 notes that `transport/websocket/handler.go` needs to call RoomManager, creating a Layer 2 → Layer 2.5 dependency. The solution is a `SessionHandler` interface. But WHERE this interface is defined matters:

- If defined in `service/` → transport imports service → bad (Layer 2 importing Layer 2.5+)
- If defined in `transport/` → RoomManager implements an interface in a package it doesn't know about → acceptable in Go (implicit interfaces) but unclear
- If defined in a shared location → what shared location?

### Decision: Define in `transport/interfaces.go` (D12)

```go
// pkg/transport/interfaces.go

// SessionHandler is the contract that transports use to initiate
// and manage participant sessions. Implemented by RoomManager.
//
// Defined here (in transport/) because this is the CONSUMER's package.
// Go's implicit interface satisfaction means RoomManager in service/
// does not need to import this package — it just needs matching methods.
type SessionHandler interface {
    StartSession(ctx context.Context, req *SessionRequest) (*SessionResponse, error)
    RemoveParticipant(roomName HubLive.RoomName, identity HubLive.ParticipantIdentity) error
}

// SessionRequest contains all info needed to start a participant session.
type SessionRequest struct {
    RoomName    HubLive.RoomName
    Identity    HubLive.ParticipantIdentity
    Reconnect   bool
    // ... other fields extracted from current rtcservice.go startSession params
}

type SessionResponse struct {
    Room        *HubLive.Room
    Participant *HubLive.ParticipantInfo
    // ... other fields
}
```

**Why this works in Go:**
```go
// service/roommanager.go — does NOT import transport/
// Go implicit interface: RoomManager satisfies transport.SessionHandler
// if it has matching method signatures.

func (rm *RoomManager) StartSession(ctx context.Context, req *transport.SessionRequest) (*transport.SessionResponse, error) {
    // Wait — this imports transport.SessionRequest → circular?
}
```

**Problem:** If `SessionRequest` is defined in `transport/`, then `roommanager.go` must import `transport/` to use it. But `roommanager.go` is at Layer 2.5 and `transport/` is at Layer 2 — this is fine (higher layer importing lower). No circular dependency.

```
transport/interfaces.go defines SessionHandler, SessionRequest
transport/websocket/handler.go uses SessionHandler (same package parent)
service/roommanager.go imports transport.SessionRequest, implements SessionHandler
service/wire.go binds: wire.Bind(new(transport.SessionHandler), new(*RoomManager))
```

**Dependency check:**
```
transport/ → nothing (defines interfaces only)        ✅
service/roommanager.go → transport/ (for SessionRequest type)  ✅ (Layer 2.5 → Layer 2)
transport/websocket/ → transport/ (for SessionHandler interface)  ✅ (same package)
```

No circular dependency. Clean layer compliance.

### Add to Decision Log

| # | Decision | Alternatives Considered | Rationale |
|---|----------|------------------------|-----------|
| D12 | SessionHandler interface defined in `transport/interfaces.go` | (a) Define in `service/`, (b) Define in new `shared/` package, (c) Define in `domain/` | Consumer-side interface definition is idiomatic Go. transport/ is Layer 2; service/ (Layer 2.5) can import it without cycle. No need for extra shared package. |

---

## Tổng kết Bổ sung

Bản bổ sung này giải quyết **10 vấn đề** đã phát hiện trong review lần 1, thêm **10 sections mới** trong bản bổ sung v1, và thêm **5 sections mới** trong bản bổ sung v2:

**Fixes (10) — v1:**
1. S1 — Dependency rule violation: SFU reclassified to Layer 1
2. S2 — rtc/types/ migration: phased alias strategy
3. S3 — Shared mutable state: SharedContext pattern + lock ordering
4. S4 — RoomManager layer: Application Service (Layer 2.5) + IOService clarification
5. S5 — Phase 2/3 coupling: split into 2A/2B, dependency map, basic_auth.go included
6. S6 — "Zero behavior changes": corrected to acknowledge 3 minor changes
7. S7 — Rollback strategy: per-phase procedures + canary deployment
8. S8 — File count: reconciled 31 files with complete inventory + clientconfiguration analysis
9. S9 — signal.go: clarified destination and contents
10. S10 — SubscriptionPolicy: hot path acknowledged, benchmark required

**New sections (10) — v1:**
11. S11 — Test strategy: 29 new tests + 2 benchmarks across all phases
12. S12 — Circular import detection: automated script + CI integration
13. S13 — Wire DI migration: concrete provider set changes per phase
14. S14 — Transition aliases: deprecation schedule + removal timeline
15. S15 — Concurrency safety audit: lock audit checklist per phase
16. S16 — Revised dependency flow: 5-layer model with explicit rules
17. S17 — Revised migration phases: 2A/2B split + additional steps
18. S18 — Revised verification checklist: comprehensive per-phase + final
19. S19 — Revised risk matrix: 17 risks (was 7)
20. S20 — Decision log: 10 architectural decisions with rationale

**New sections (5) — v2:**
21. S21 — External consumer audit: verify HubLive-cli/egress/ingress imports before Phase 1
22. S22 — Protobuf types in domain: use directly (D11), no mapping layer
23. S23 — Error handling: sentinel error migration strategy + audit script
24. S24 — Benchmark baseline: pre-refactor benchmark suite + per-phase comparison
25. S25 — SessionHandler interface location: defined in transport/ (D12), no circular deps

**Updated Decision Log:** D1-D12 (was D1-D10)

**New Phase 0 added:** External audit + error audit + benchmark baseline — MUST complete before Phase 1.
