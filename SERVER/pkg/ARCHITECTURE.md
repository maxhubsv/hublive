# HubLive Server — Package Architecture

## Layer Model

```
Layer 0 (Foundation):  utils/, config/, metric/, clientconfiguration/
Layer 1 (Core):        sfu/ (media engine), domain/ (interfaces + extension points)
Layer 2 (Infra):       store/, transport/, routing/, telemetry/, rtc/ (implementations)
Layer 2.5 (App):       service/roommanager, service/roomallocator, service/ioservice
Layer 3 (API):         api/middleware/ (auth), service/ Twirp handlers
Layer 4 (Bootstrap):   service/server, service/wire, service/turn
```

**Rule:** Layer N imports Layer 0..N-1 only.

**Known exceptions:**
- `rtc/` (Layer 2) imports `sfu/` (Layer 1) — expected, SFU is the media engine
- `rtc/` imports `routing/`, `telemetry/`, `config/` — legitimate infrastructure dependencies
- `service/` Twirp handlers share utilities with transport code — tightly coupled, to be extracted in future

## Package Responsibilities

### pkg/domain/ — Extension Point Interfaces (Layer 1)

Pure interfaces with zero infrastructure dependencies. Imports only `rtc/types/`.

- **domain/room/** — `Room` type alias, `SubscriptionPolicy` interface, `DefaultSubscriptionPolicy`
- **domain/participant/** — `Participant`, `LocalParticipant` type aliases
- **domain/track/** — `MediaTrack`, `SubscribedTrack`, `DataTrack` type aliases
- **domain/agent/** — `AgentStore`, `AgentDispatcher`, `WorkerRegistry` interfaces + `AgentJob`, `WorkerInfo` types
  - `AgentStore` breaks the documented `rtc → service → rtc` import cycle

### pkg/store/ — Storage Abstraction (Layer 2)

- **store/interfaces.go** — `ObjectStore`, `ServiceStore`, `OSSServiceStore`, `EgressStore`, `IngressStore`, `SIPStore`, `AgentStore`
- **store/errors.go** — Store-related sentinel errors (`ErrRoomNotFound`, `ErrParticipantNotFound`, etc.)
- **store/local/** — In-memory implementation (single-node mode)
- **store/redis/** — Redis implementation (distributed mode) with all Redis key constants

### pkg/transport/ — Protocol Handlers (Layer 2)

Zero imports from `service/` — fully independent.

- **transport/interfaces.go** — `Transport` interface (implement to add new protocols)
- **transport/registry.go** — `TransportRegistry` with `Register()`, `SetupRoutes()`, `GetAll()`
- **transport/signal.go** — `SessionHandler` interface, `SignalServer`, signal relay internals

### pkg/api/ — API Layer (Layer 3)

- **api/middleware/auth.go** — `APIKeyAuthMiddleware`, `ErrorHandler` callback pattern, all `Ensure*Permission` functions, auth sentinel errors
- **api/middleware/basic_auth.go** — `GenBasicAuthMiddleware` for Prometheus

### pkg/rtc/ — RTC Implementation (Layer 2)

Concrete implementations of domain interfaces. Depends on `sfu/`, `routing/`, `telemetry/`, `config/`.

**File organization (god objects split for readability):**

Participant (~4250 lines → 5 files):
- `participant.go` (2459) — Core state, lifecycle, transport, migration, data messages
- `participant_publisher.go` (1505) — Publishing, track management, pending tracks, codecs
- `participant_subscriber.go` (340) — Subscribing, permissions, RTCP worker, quality
- `participant_signal.go` (374) — Signal message sending
- `participant_sdp.go` (408) — SDP negotiation helpers

Transport (~3232 lines → 3 files):
- `transport.go` (1284) — Core: PCTransport struct, lifecycle, data channels, connection state
- `transport_negotiation.go` (1313) — SDP offer/answer, transceiver management, codec config
- `transport_ice.go` (712) — ICE candidates, TURN, ICE restart, connection stats

Other key files:
- `room.go` — Room state, participant management, auto-subscription
- `mediatrack.go`, `subscribedtrack.go`, etc. — Track implementations
- `transportmanager.go` — Transport coordination

### pkg/service/ — Application Services + Bootstrap (Layer 2.5-4)

- **roommanager.go** — Room lifecycle orchestration (creates rooms, manages sessions)
- **roomallocator.go** — Node selection for room placement
- **ioservice.go** — Egress/Ingress/SIP IO bridge
- **server.go** — HTTP server bootstrap, middleware chain, route mounting
- **wire.go / wire_gen.go** — Google Wire dependency injection
- **turn.go** — Embedded TURN/STUN server
- **Twirp handlers** — roomservice.go, egress.go, ingress.go, sip.go, agentservice.go, agent_dispatch_service.go
- **Transition files** — compat_store.go, auth.go, basic_auth.go, signal.go (aliases to new locations)

### pkg/sfu/ — Media Engine (Layer 1) — DO NOT MODIFY

13 sub-packages, well-organized. Audio/video processing, bandwidth estimation, stream allocation, codec handling.

## Extension Points

### 1. Transport Protocol
Implement `transport.Transport`, register via `TransportRegistry`:
```go
type MyTransport struct{}
func (m MyTransport) Protocol() string { return "my-protocol" }
func (m MyTransport) SetupRoutes(mux *http.ServeMux) { mux.HandleFunc("/my", m.handle) }

registry.Register(MyTransport{})
```

### 2. Storage Backend
Implement `store.ObjectStore` (or sub-interfaces like `store.EgressStore`):
```go
type PostgresStore struct{ db *sql.DB }
// Implement all ObjectStore methods...
```

### 3. Subscription Policy
Implement `domain/room.SubscriptionPolicy`:
```go
type RoleBasedPolicy struct{}
func (p RoleBasedPolicy) ShouldSubscribe(sub types.LocalParticipant, track types.MediaTrack) bool {
    // Custom logic: only subscribe if roles match
}
```

### 4. Agent Dispatch
Implement `domain/agent.AgentDispatcher`:
```go
type PriorityDispatcher struct{}
func (d PriorityDispatcher) Dispatch(ctx context.Context, job *agent.AgentJob) error {
    // Custom dispatch logic with priority queues
}
```

### 5. Worker Registry
Implement `domain/agent.WorkerRegistry`:
```go
type ExternalWorkerPool struct{}
func (p ExternalWorkerPool) FindWorkers(jobType HubLive.JobType, ns string) ([]*agent.WorkerInfo, error) {
    // Query external worker pool
}
```

## Transition Aliases

Files in `service/` contain type aliases for backward compatibility:
- `service/compat_store.go` — Store type/constructor aliases
- `service/auth.go` — Auth function aliases (wraps `api/middleware/`)
- `service/basic_auth.go` — BasicAuth alias
- `service/signal.go` — Signal type aliases + `NewDefaultSignalServer`

All aliases are marked `// Deprecated`. They exist because Twirp handlers and Wire DI
remain in `service/` and reference these types by unqualified name.

## Dependency Verification

Run `scripts/check-imports.sh` to verify:
- No circular imports
- `domain/` imports only `rtc/types/` (no infrastructure)
- `transport/` imports no `service/`
- `api/` imports no `service/`
- Layer dependency matrix

## Testing

Extension points are verified by `pkg/domain/verify_test.go` which provides
compile-time interface satisfaction checks for all 6 extension point interfaces.
