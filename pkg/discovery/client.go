package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

type EnvoyServer struct {
	nodeID        string
	snapshotCache cache.SnapshotCache
}

type EnvoyServerParams struct {
	NodeID string
}

func NewEnvoyServer(params EnvoyServerParams) *EnvoyServer {
	cache := cache.NewSnapshotCache(false, cache.IDHash{}, nil)
	return &EnvoyServer{
		snapshotCache: cache,
		nodeID:        params.NodeID,
	}
}

func (es *EnvoyServer) Serve(ctx context.Context) error {
	// Create the snapshot that we'll serve to Envoy
	simp := SimpleEnvoyConfig{
		ClusterName: ClusterName,
		Port:        uint32(1000),
		Path: SimpleEnvoyPath{
			Value: "/",
			Type:  SimpleEnvoyPathTypePrefix,
		},
	}

	params := GenerateSnapshotParams{
		Version:            "1",
		SimpleEnvoyConfigs: []SimpleEnvoyConfig{simp},
		ListenerName:       ListenerName,
		RouteName:          RouteName,
	}

	snapshot := GenerateSnapshot(params)
	if err := snapshot.Consistent(); err != nil {
		return err
	}

	// Add the snapshot to the cache
	if err := es.snapshotCache.SetSnapshot(ctx, es.nodeID, *snapshot); err != nil {
		return err
	}

	cb := &test.Callbacks{}
	srv := server.NewServer(ctx, es.snapshotCache, cb)
	RunServer(ctx, srv, 18000)

	return nil
}

func (es *EnvoyServer) GetSnapshotCache() cache.SnapshotCache {
	return es.snapshotCache
}

const (
	grpcKeepaliveTime        = 30 * time.Second
	grpcKeepaliveTimeout     = 5 * time.Second
	grpcKeepaliveMinTime     = 30 * time.Second
	grpcMaxConcurrentStreams = 1000000
)

func registerServer(grpcServer *grpc.Server, server server.Server) {
	// register services
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, server)
	secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, server)
	runtimeservice.RegisterRuntimeDiscoveryServiceServer(grpcServer, server)
}

// RunServer starts an xDS server at the given port.
func RunServer(ctx context.Context, srv server.Server, port uint) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems. Keepalive timeouts based on connection_keepalive parameter https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/examples#dynamic
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions,
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    grpcKeepaliveTime,
			Timeout: grpcKeepaliveTimeout,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             grpcKeepaliveMinTime,
			PermitWithoutStream: true,
		}),
	)
	grpcServer := grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal(err)
	}

	registerServer(grpcServer, srv)

	log.Printf("management server listening on %d\n", port)
	if err = grpcServer.Serve(lis); err != nil {
		log.Println(err)
	}
}
