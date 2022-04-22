package discovery

import (
	"context"
	"strings"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	v32 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	ClusterName       = "init_cluster"
	RouteName         = "local_route"
	ListenerNameHTTP  = "listener_http"
	ListenerPortHTTP  = 80
	ListenerNameHTTPS = "listener_https"
	ListenerPortHTTPS = 443
	UpstreamHost      = "localhost"
	UpstreamPort      = 18080
)

func makeCluster(clusterName string, port uint32, endpoints []string) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STATIC},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment:       makeEndpoint(clusterName, port, endpoints),
		DnsLookupFamily:      cluster.Cluster_V4_ONLY,
	}
}

func makeEndpoint(clusterName string, port uint32, endpoints []string) *endpoint.ClusterLoadAssignment {

	lbendpoints := []*endpoint.LbEndpoint{}

	for _, ep := range endpoints {
		lbendpoints = append(lbendpoints, &endpoint.LbEndpoint{
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_TCP,
								Address:  ep,
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: port,
								},
							},
						},
					},
				},
			},
		})
	}

	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: lbendpoints,
		}},
	}

}

func makeRoute(routeName string, simpMap map[string][]SimpleEnvoyConfig) *route.RouteConfiguration {

	virtualHosts := []*route.VirtualHost{}
	for key, value := range simpMap {
		routes := []*route.Route{}
		for _, simp := range value {
			match := &route.RouteMatch{}
			if simp.Path.Type == SimpleEnvoyPathTypeMatch {
				match.PathSpecifier = &route.RouteMatch_Path{
					Path: simp.Path.Value,
				}
			} else {
				match.PathSpecifier = &route.RouteMatch_Prefix{
					Prefix: simp.Path.Value,
				}
			}

			action := &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: simp.ClusterName,
					},
				},
			}

			if simp.Path.RewriteRegex != "" {
				regexRewrite := &v32.RegexMatchAndSubstitute{
					Pattern: &v32.RegexMatcher{
						Regex: simp.Path.Value,
						EngineType: &v32.RegexMatcher_GoogleRe2{
							GoogleRe2: &v32.RegexMatcher_GoogleRE2{},
						},
					},
					Substitution: simp.Path.RewriteRegex,
				}

				action.Route.RegexRewrite = regexRewrite
			}

			routes = append(routes, &route.Route{
				Match:  match,
				Action: action,
			})
		}

		name := "wildcard_service"
		if key != "*" {
			partialReplacement := strings.ReplaceAll(key, ".", "_")
			name = strings.ReplaceAll(partialReplacement, "*", "wildcard")
		}

		virtualHosts = append(virtualHosts, &route.VirtualHost{
			Name:    name,
			Domains: []string{key},
			Routes:  routes,
		})
	}

	return &route.RouteConfiguration{
		Name:         routeName,
		VirtualHosts: virtualHosts,
	}
}

func makeHTTPListener(listenerName string, listenerPort uint32, route string) (*listener.Listener, error) {
	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    makeConfigSource(),
				RouteConfigName: route,
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}
	pbst, err := anypb.New(manager)
	if err != nil {
		return nil, err
	}

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: listenerPort,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}},
	}, nil
}

func makeHTTPSListener(listenerName string, listenerPort uint32, route string, tlsConfigs []TLSConfig) (*listener.Listener, error) {
	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "https",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    makeConfigSource(),
				RouteConfigName: route,
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}
	pbst, err := anypb.New(manager)
	if err != nil {
		return nil, err
	}

	tlsCerts := []*auth.TlsCertificate{}
	for _, tlsConfig := range tlsConfigs {
		tlsCerts = append(tlsCerts, &auth.TlsCertificate{
			PrivateKey: &core.DataSource{
				Specifier: &core.DataSource_InlineString{InlineString: tlsConfig.PrivateKeyString},
			},
			CertificateChain: &core.DataSource{
				Specifier: &core.DataSource_InlineString{InlineString: tlsConfig.CertificateString},
			},
		})
	}

	tlsc := &auth.DownstreamTlsContext{
		CommonTlsContext: &auth.CommonTlsContext{
			TlsCertificates: tlsCerts,
		},
	}

	pbtlsc, err := anypb.New(tlsc)
	if err != nil {
		return nil, err
	}

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: listenerPort,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
			TransportSocket: &core.TransportSocket{
				Name: wellknown.TransportSocketTLS,
				ConfigType: &core.TransportSocket_TypedConfig{
					TypedConfig: pbtlsc,
				},
			},
		}},
	}, nil
}

func makeConfigSource() *core.ConfigSource {
	source := &core.ConfigSource{}
	source.ResourceApiVersion = resource.DefaultAPIVersion
	source.ConfigSourceSpecifier = &core.ConfigSource_ApiConfigSource{
		ApiConfigSource: &core.ApiConfigSource{
			TransportApiVersion:       resource.DefaultAPIVersion,
			ApiType:                   core.ApiConfigSource_GRPC,
			SetNodeOnFirstMessageOnly: true,
			GrpcServices: []*core.GrpcService{{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{ClusterName: "xds_cluster"},
				},
			}},
		},
	}
	return source
}

type SimpleEnvoyPathType string

const (
	SimpleEnvoyPathTypePrefix SimpleEnvoyPathType = "Prefix"
	SimpleEnvoyPathTypeMatch  SimpleEnvoyPathType = "Match"
)

type SimpleEnvoyPath struct {
	Value        string
	Type         SimpleEnvoyPathType
	RewriteRegex string
}

type SimpleEnvoyConfig struct {
	Port        uint32
	ClusterName string
	Path        SimpleEnvoyPath
	Endpoints   []string
}

type GenerateSnapshotParams struct {
	Version              string
	SimpleEnvoyConfigMap map[string][]SimpleEnvoyConfig
	RouteName            string
	ListenerConfigs      []SimpleListenerConfig
}

type HTTPListenerConfig struct {
	ListenerName string
	ListenerPort uint32
}

type TLSConfig struct {
	CertificateString string
	PrivateKeyString  string
}

type SimpleListenerConfig struct {
	ListenerName string
	ListenerPort uint32
	TLSConfig    *[]TLSConfig
}

func GenerateSnapshot(params GenerateSnapshotParams) (*cache.Snapshot, error) {

	snapShotMap := make(map[resource.Type][]types.Resource)

	for _, simpList := range params.SimpleEnvoyConfigMap {
		for _, simp := range simpList {
			snapShotMap[resource.ClusterType] = append(snapShotMap[resource.ClusterType], makeCluster(simp.ClusterName, simp.Port, simp.Endpoints))
		}
	}

	snapShotMap[resource.RouteType] = append(snapShotMap[resource.RouteType], makeRoute(params.RouteName, params.SimpleEnvoyConfigMap))

	for _, listenerConfig := range params.ListenerConfigs {
		listener, err := makeListener(listenerConfig, params.RouteName)
		if err != nil {
			return nil, err
		}

		snapShotMap[resource.ListenerType] = append(snapShotMap[resource.ListenerType], listener)
	}

	snap, err := cache.NewSnapshot(params.Version, snapShotMap)
	if err != nil {
		return nil, err
	}

	return &snap, nil
}

func makeListener(listenerConfig SimpleListenerConfig, routeName string) (*listener.Listener, error) {
	var listener *listener.Listener
	var err error
	if listenerConfig.TLSConfig == nil {
		listener, err = makeHTTPListener(
			listenerConfig.ListenerName,
			listenerConfig.ListenerPort,
			routeName,
		)
		if err != nil {
			return nil, err
		}
	} else {
		listener, err = makeHTTPSListener(
			listenerConfig.ListenerName,
			listenerConfig.ListenerPort,
			routeName,
			*listenerConfig.TLSConfig)
		if err != nil {
			return nil, err
		}
	}

	return listener, nil
}

func SetSnapshot(ctx context.Context, nodeID string, snapshotCache cache.SnapshotCache, snapshot *cache.Snapshot) error {
	return snapshotCache.SetSnapshot(ctx, nodeID, *snapshot)
}
