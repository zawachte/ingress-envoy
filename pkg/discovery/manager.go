package discovery

import (
	"context"
	"log"
	"sync"

	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
)

type EnvoyManager struct {
	Signal         chan struct{}
	Debug          bool
	Fetches        int
	Requests       int
	DeltaRequests  int
	DeltaResponses int
	mu             sync.Mutex
	Nodes          map[string][]int64
	snapshotCache  cache.SnapshotCache
}

func newEnvoyManager() *EnvoyManager {
	cache := cache.NewSnapshotCache(false, cache.IDHash{}, nil)
	return &EnvoyManager{
		Nodes:         make(map[string][]int64),
		snapshotCache: cache,
	}
}

func (em *EnvoyManager) GetSnapshotCache() cache.SnapshotCache {
	return em.snapshotCache
}

func (em *EnvoyManager) Report() {
	em.mu.Lock()
	defer em.mu.Unlock()
	log.Printf("server callbacks fetches=%d requests=%d\n", em.Fetches, em.Requests)
}
func (em *EnvoyManager) OnStreamOpen(_ context.Context, id int64, typ string) error {
	em.mu.Lock()
	defer em.mu.Unlock()
	if em.Debug {
		log.Printf("stream %d open for %s\n", id, typ)
	}
	return nil
}
func (em *EnvoyManager) OnStreamClosed(id int64) {
	em.mu.Lock()
	defer em.mu.Unlock()
	if em.Debug {
		log.Printf("stream %d closed\n", id)
	}
}
func (em *EnvoyManager) OnDeltaStreamOpen(_ context.Context, id int64, typ string) error {
	if em.Debug {
		log.Printf("delta stream %d open for %s\n", id, typ)
	}
	return nil
}
func (em *EnvoyManager) OnDeltaStreamClosed(id int64) {
	if em.Debug {
		log.Printf("delta stream %d closed\n", id)
	}
}
func (em *EnvoyManager) OnStreamRequest(stream int64, dr *discovery.DiscoveryRequest) error {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.Requests++
	if em.Signal != nil {
		close(em.Signal)
		em.Signal = nil
	}

	_, ok := em.Nodes[dr.Node.Id]
	if ok {
		em.Nodes[dr.Node.Id] = append(em.Nodes[dr.Node.Id], stream)
	} else {
		em.Nodes[dr.Node.Id] = []int64{stream}
	}

	return nil
}
func (em *EnvoyManager) OnStreamResponse(context.Context, int64, *discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {
}
func (em *EnvoyManager) OnStreamDeltaResponse(id int64, req *discovery.DeltaDiscoveryRequest, res *discovery.DeltaDiscoveryResponse) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.DeltaResponses++
}
func (em *EnvoyManager) OnStreamDeltaRequest(id int64, req *discovery.DeltaDiscoveryRequest) error {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.DeltaRequests++
	if em.Signal != nil {
		close(em.Signal)
		em.Signal = nil
	}

	return nil
}
func (em *EnvoyManager) OnFetchRequest(_ context.Context, req *discovery.DiscoveryRequest) error {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.Fetches++
	if em.Signal != nil {
		close(em.Signal)
		em.Signal = nil
	}
	return nil
}
func (em *EnvoyManager) OnFetchResponse(*discovery.DiscoveryRequest, *discovery.DiscoveryResponse) {}

func (em *EnvoyManager) PushChanges(ctx context.Context, snapshot cache.Snapshot) error {
	for n := range em.Nodes {
		err := em.snapshotCache.SetSnapshot(ctx, n, snapshot)
		if err != nil {
			return err
		}
	}

	return nil
}
