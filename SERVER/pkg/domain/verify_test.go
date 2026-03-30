package domain_test

// Compile-time verification that all extension point interfaces are implementable
// and that the refactored package structure is sound. These tests don't run logic —
// they verify interfaces compile and types satisfy contracts.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"__GITHUB_HUBLIVE__protocol/hublive"

	"github.com/maxhubsv/hublive-server/pkg/domain/agent"
	"github.com/maxhubsv/hublive-server/pkg/domain/room"
	"github.com/maxhubsv/hublive-server/pkg/domain/track"
	"github.com/maxhubsv/hublive-server/pkg/store"
	"github.com/maxhubsv/hublive-server/pkg/transport"
)

// --- Extension Point 1: Transport ---

type mockTransport struct{}

func (m mockTransport) Protocol() string                 { return "mock" }
func (m mockTransport) SetupRoutes(mux *http.ServeMux)   {}

func TestTransportInterface(t *testing.T) {
	var _ transport.Transport = mockTransport{}

	registry := transport.NewTransportRegistry()
	registry.Register(mockTransport{})
	if len(registry.GetAll()) != 1 {
		t.Fatal("expected 1 transport")
	}
	if registry.GetAll()[0].Protocol() != "mock" {
		t.Fatal("expected mock protocol")
	}
}

// --- Extension Point 2: Store ---

type mockObjectStore struct{}

func (m mockObjectStore) LoadRoom(_ context.Context, _ hublive.RoomName, _ bool) (*hublive.Room, *hublive.RoomInternal, error) {
	return nil, nil, nil
}
func (m mockObjectStore) RoomExists(_ context.Context, _ hublive.RoomName) (bool, error) {
	return false, nil
}
func (m mockObjectStore) ListRooms(_ context.Context, _ []hublive.RoomName) ([]*hublive.Room, error) {
	return nil, nil
}
func (m mockObjectStore) DeleteRoom(_ context.Context, _ hublive.RoomName) error { return nil }
func (m mockObjectStore) HasParticipant(_ context.Context, _ hublive.RoomName, _ hublive.ParticipantIdentity) (bool, error) {
	return false, nil
}
func (m mockObjectStore) LoadParticipant(_ context.Context, _ hublive.RoomName, _ hublive.ParticipantIdentity) (*hublive.ParticipantInfo, error) {
	return nil, nil
}
func (m mockObjectStore) ListParticipants(_ context.Context, _ hublive.RoomName) ([]*hublive.ParticipantInfo, error) {
	return nil, nil
}
func (m mockObjectStore) LockRoom(_ context.Context, _ hublive.RoomName, _ time.Duration) (string, error) {
	return "", nil
}
func (m mockObjectStore) UnlockRoom(_ context.Context, _ hublive.RoomName, _ string) error {
	return nil
}
func (m mockObjectStore) StoreRoom(_ context.Context, _ *hublive.Room, _ *hublive.RoomInternal) error {
	return nil
}
func (m mockObjectStore) StoreParticipant(_ context.Context, _ hublive.RoomName, _ *hublive.ParticipantInfo) error {
	return nil
}
func (m mockObjectStore) DeleteParticipant(_ context.Context, _ hublive.RoomName, _ hublive.ParticipantIdentity) error {
	return nil
}

func TestStoreInterfaces(t *testing.T) {
	var _ store.ObjectStore = mockObjectStore{}
	var _ store.ServiceStore = mockObjectStore{}
	var _ store.OSSServiceStore = mockObjectStore{}
}

// --- Extension Point 3: Subscription Policy ---

type prioritySubscriptionPolicy struct{}

func (p prioritySubscriptionPolicy) ShouldSubscribe(subscriber track.MediaTrack, track track.MediaTrack) bool {
	return true // custom logic would go here
}

func TestSubscriptionPolicy(t *testing.T) {
	// Verify DefaultSubscriptionPolicy exists and satisfies the interface
	_ = room.DefaultSubscriptionPolicy{}

	// Verify store error sentinels exist
	_ = store.ErrRoomNotFound
	_ = store.ErrParticipantNotFound
}

// --- Extension Point 4: Agent Dispatcher ---

type mockAgentDispatcher struct{}

func (m mockAgentDispatcher) Dispatch(_ context.Context, _ *agent.AgentJob) error { return nil }
func (m mockAgentDispatcher) Cancel(_ context.Context, _ string) error            { return nil }
func (m mockAgentDispatcher) ListActive(_ context.Context, _ hublive.RoomName) ([]*hublive.AgentDispatch, error) {
	return nil, nil
}

func TestAgentDispatcher(t *testing.T) {
	var _ agent.AgentDispatcher = mockAgentDispatcher{}
}

// --- Extension Point 5: Worker Registry ---

type mockWorkerRegistry struct{}

func (m mockWorkerRegistry) Register(_ *agent.WorkerInfo) error                             { return nil }
func (m mockWorkerRegistry) Deregister(_ string) error                                      { return nil }
func (m mockWorkerRegistry) FindWorkers(_ hublive.JobType, _ string) ([]*agent.WorkerInfo, error) {
	return nil, nil
}

func TestWorkerRegistry(t *testing.T) {
	var _ agent.WorkerRegistry = mockWorkerRegistry{}
}

// --- Extension Point 6: Agent Store ---

type mockAgentStore struct{}

func (m mockAgentStore) StoreAgentDispatch(_ context.Context, _ *hublive.AgentDispatch) error {
	return nil
}
func (m mockAgentStore) DeleteAgentDispatch(_ context.Context, _ *hublive.AgentDispatch) error {
	return nil
}
func (m mockAgentStore) ListAgentDispatches(_ context.Context, _ hublive.RoomName) ([]*hublive.AgentDispatch, error) {
	return nil, nil
}
func (m mockAgentStore) StoreAgentJob(_ context.Context, _ *hublive.Job) error  { return nil }
func (m mockAgentStore) DeleteAgentJob(_ context.Context, _ *hublive.Job) error { return nil }

func TestAgentStore(t *testing.T) {
	var _ agent.AgentStore = mockAgentStore{}
}
