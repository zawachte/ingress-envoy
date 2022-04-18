package discovery

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_makeCluster(t *testing.T) {

	type args struct {
		clusterName string
		port        uint32
		endpoints   []string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "simple cluster",
			args: args{
				clusterName: "",
				port:        100,
				endpoints:   []string{},
			},
		},
		{
			name: "simple cluster 2",
			args: args{
				clusterName: "",
				port:        100,
				endpoints:   []string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cluster := makeCluster(tt.args.clusterName, tt.args.port, tt.args.endpoints)
			g.Expect(cluster.Name).To(Equal(tt.args.clusterName))
		})
	}
}

func Test_makeEndpoint(t *testing.T) {
	type args struct {
		clusterName string
		port        uint32
		endpoints   []string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "simple endpoint",
			args: args{
				clusterName: "",
				port:        100,
				endpoints:   []string{},
			},
		},
		{
			name: "simple endpoint 2",
			args: args{
				clusterName: "",
				port:        100,
				endpoints:   []string{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			clusterLoadAssignment := makeEndpoint(tt.args.clusterName, tt.args.port, tt.args.endpoints)
			g.Expect(clusterLoadAssignment.ClusterName).To(Equal(tt.args.clusterName))
		})
	}
}

func Test_makeRoute(t *testing.T) {
	type args struct {
		routeName string
		simpMap   map[string][]SimpleEnvoyConfig
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "simple route",
			args: args{
				routeName: "",
			},
		},
		{
			name: "simple route 2",
			args: args{
				routeName: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			routeConfig := makeRoute(tt.args.routeName, tt.args.simpMap)
			g.Expect(routeConfig.Name).To(Equal(tt.args.routeName))
		})
	}

}

func Test_makeHTTPListener(t *testing.T) {
	type args struct {
		listenerName string
		route        string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "simple listener",
			args: args{
				listenerName: "",
				route:        "",
			},
		},
		{
			name: "simple listener 2",
			args: args{
				listenerName: "",
				route:        "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			listener := makeHTTPListener(tt.args.listenerName, tt.args.route)
			g.Expect(listener.Name).To(Equal(tt.args.listenerName))
		})
	}
}

func Test_GenerateSnapshot(t *testing.T) {
	type args struct {
		params GenerateSnapshotParams
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "simple snapshot",
			args: args{
				params: GenerateSnapshotParams{},
			},
		},
		{
			name: "simple route 2",
			args: args{
				params: GenerateSnapshotParams{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			snapshot := GenerateSnapshot(tt.args.params)
			g.Expect(snapshot.Consistent()).NotTo(HaveOccurred())
		})
	}
}
