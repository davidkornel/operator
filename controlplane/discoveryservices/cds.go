package discoveryservices

import (
	"context"
	"fmt"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	"github.com/davidkornel/operator/state"
	pb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoyservicediscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/golang/protobuf/ptypes/any"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"io"
	types2 "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

type clusterDiscoveryService struct {
	pb.UnimplementedClusterDiscoveryServiceServer
}

func (c clusterDiscoveryService) StreamClusters(server clusterservice.ClusterDiscoveryService_StreamClustersServer) error {
	panic("implement me")
}

func (c clusterDiscoveryService) FetchClusters(ctx context.Context, request *envoyservicediscoveryv3.DiscoveryRequest) (*envoyservicediscoveryv3.DiscoveryResponse, error) {
	panic("implement me")
}

func (c clusterDiscoveryService) DeltaClusters(server clusterservice.ClusterDiscoveryService_DeltaClustersServer) error {
	logger := ctrl.Log.WithName("CDS")
	logger.Info("CDS INIT")
	for {
		//ddr delta discovery request
		ddr, err := server.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		logger.Info("Request:", "Node-id: ", ddr.Node.Id)
		if _, ok := state.CdsChannels[ddr.Node.Id]; !ok {
			state.CdsChannels[ddr.Node.Id] = make(chan state.SignalMessageOnCdsChannels)
			logger.Info("Channel has been added for", "uid", ddr.Node.Id)
		}
		cdsMessage, isOpen := <-state.CdsChannels[ddr.Node.Id]
		if !isOpen {
			//TODO Close connection from serverside
			logger.Info("gRPC connection should have ended here")
		}
		switch cdsMessage.Verb {

		case state.Add:
			clusters := cdsMessage.Resources
			//logger.Info("clusters", "c", clusters)
			err = server.Send(createClusterDeltaDiscoveryResponse(clusters, nil))
			if err != nil {
				logger.Error(err, "Error occurred while sending cluster configuration to envoy")
				return err
			}

		case state.Delete:
			clustersToBeDeleted := make([]string, 0)
			for _, c := range cdsMessage.Resources {
				clustersToBeDeleted = append(clustersToBeDeleted, c.Name)
			}
			err = server.Send(createClusterDeltaDiscoveryResponse(nil, clustersToBeDeleted))
			if err != nil {
				logger.Error(err, "Error occurred while REMOVING cluster configuration to envoy")
				return err
			}

		case state.Change:
			panic("change not implemented")
			//TODO implement
		}
	}
}

func createClusterDeltaDiscoveryResponse(clustersToBeAdded []*cluster.Cluster, clustersToBeDeleted []string) *envoyservicediscoveryv3.DeltaDiscoveryResponse {
	var resources []*envoyservicediscoveryv3.Resource
	for _, c := range clustersToBeAdded {
		r := &envoyservicediscoveryv3.Resource{
			Name:     c.Name,
			Version:  "1",
			Resource: ConvertClustersToAny(c),
		}
		resources = append(resources, r)
	}

	response := envoyservicediscoveryv3.DeltaDiscoveryResponse{
		SystemVersionInfo: "Testversion",
		Resources:         resources,
		TypeUrl:           "type.googleapis.com/envoy.config.cluster.v3.Cluster",
		RemovedResources:  clustersToBeDeleted,
		Nonce:             "cluster",
	}

	return &response
}

func ConvertClustersToAny(c *cluster.Cluster) *any.Any {
	clusterAny, _ := anypb.New(c)
	return clusterAny
}

func NewCdsServer() *clusterDiscoveryService {
	s := &clusterDiscoveryService{
		UnimplementedClusterDiscoveryServiceServer: pb.UnimplementedClusterDiscoveryServiceServer{},
	}
	logger := ctrl.Log.WithName("CDS server")
	logger.Info("init")
	return s
}

func CreateEnvoyClusterConfigFromVsvcSpec(spec l7mpiov1.VirtualServiceSpec) []*cluster.Cluster {
	var clusters []*cluster.Cluster
	for _, l := range spec.Listeners {

		c := &cluster.Cluster{
			Name:           l.Udp.Cluster.Name,
			ConnectTimeout: durationpb.New(5 * time.Second),
			//LbConfig: &cluster.Cluster_MaglevLbConfig_{
			//	MaglevLbConfig: &cluster.Cluster_MaglevLbConfig{
			//		TableSize: &wrappers.UInt64Value{Value: 5},
			//	},
			//},
			LbPolicy: cluster.Cluster_MAGLEV,
		}
		if l.Udp.Cluster.HealthCheck != nil {
			if l.Udp.Cluster.HealthCheck.Protocol == "TCP" {
				c.HealthChecks = []*core.HealthCheck{{
					Timeout:            durationpb.New(100 * time.Millisecond),
					Interval:           durationpb.New(100 * time.Millisecond),
					UnhealthyThreshold: &wrappers.UInt32Value{Value: 1},
					HealthyThreshold:   &wrappers.UInt32Value{Value: 1},
					HealthChecker: &core.HealthCheck_TcpHealthCheck_{
						TcpHealthCheck: &core.HealthCheck_TcpHealthCheck{
							Send: &core.HealthCheck_Payload{
								Payload: &core.HealthCheck_Payload_Text{
									Text: "000000FF",
								},
							},
							Receive: []*core.HealthCheck_Payload{{
								Payload: &core.HealthCheck_Payload_Text{
									Text: "000000FF",
								},
							}},
						},
					},
					NoTrafficInterval: durationpb.New(1 * time.Second),
				}}
			}
		}

		switch l.Udp.Cluster.ServiceDiscovery {
		case "strictdns":
			createStrictDnsConfig(c, l)
		case "eds":
			//TODO handle eds
			createEdsConfig(c)
		}

		clusters = append(clusters, c)
	}
	return clusters
}

func createEdsConfig(c *cluster.Cluster) {
	c.ClusterDiscoveryType = &cluster.Cluster_Type{
		Type: cluster.Cluster_EDS,
	}
	c.EdsClusterConfig = &cluster.Cluster_EdsClusterConfig{
		EdsConfig: &core.ConfigSource{
			ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
				ApiConfigSource: &core.ApiConfigSource{
					ApiType:             core.ApiConfigSource_DELTA_GRPC,
					TransportApiVersion: core.ApiVersion_V3,
					GrpcServices: []*core.GrpcService{{
						TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
								ClusterName: "xds_cluster",
							},
						},
					}},
					SetNodeOnFirstMessageOnly: false,
				},
			},
			InitialFetchTimeout: durationpb.New(600 * time.Second),
			ResourceApiVersion:  core.ApiVersion_V3,
		},
	}
}

func createStrictDnsConfig(c *cluster.Cluster, l l7mpiov1.Listener) {
	c.ClusterDiscoveryType = &cluster.Cluster_Type{
		Type: cluster.Cluster_STRICT_DNS,
	}
	c.LoadAssignment = createEndpoint(l.Udp.Cluster)
}

func createEndpoint(c l7mpiov1.Cluster) *endpoint.ClusterLoadAssignment {

	loadAssignment := &endpoint.ClusterLoadAssignment{
		ClusterName: c.Name,
	}
	lbEndpoints := make([]*endpoint.LbEndpoint, len(c.Endpoints))
	for _, e := range c.Endpoints {
		for _, a := range fillAddress(e.Host) {
			newEndpoint := &endpoint.LbEndpoint{
				HostIdentifier: &endpoint.LbEndpoint_Endpoint{
					Endpoint: &endpoint.Endpoint{
						Address: &core.Address{
							Address: &core.Address_SocketAddress{
								SocketAddress: &core.SocketAddress{
									Protocol: core.SocketAddress_UDP,
									Address:  a,
									PortSpecifier: &core.SocketAddress_PortValue{
										PortValue: e.Port,
									},
								},
							},
						},
						HealthCheckConfig: createEndpointHealthCheckConfig(e),
					},
				},
				Metadata: &core.Metadata{FilterMetadata: createStruct(a)},
			}
			lbEndpoints = append(lbEndpoints, newEndpoint)
		}
	}
	localityLbEndpoints := []*endpoint.LocalityLbEndpoints{{LbEndpoints: lbEndpoints}}
	loadAssignment.Endpoints = localityLbEndpoints
	return loadAssignment
}

func createEndpointHealthCheckConfig(e l7mpiov1.Endpoint) *endpoint.Endpoint_HealthCheckConfig {
	if e.HealthCheckPort != nil {
		hc := &endpoint.Endpoint_HealthCheckConfig{
			PortValue: *e.HealthCheckPort,
		}
		return hc
	}
	return nil
}

func createStruct(value string) map[string]*structpb.Struct {
	m, err := structpb.NewStruct(map[string]interface{}{
		"hash_key": fmt.Sprintf("%s", value),
	})
	if err != nil {
		ctrl.Log.WithName("Endpoint").Error(err, "Error occurred while running stuctpb.NewStruct")
	}
	return map[string]*_struct.Struct{"envoy.lb": m}
}

func fillAddress(host l7mpiov1.Host) []string {
	addressList := make([]string, 0)
	if host.Address != nil {
		addressList = append(addressList, *host.Address)
		return addressList
	}
	if host.Selector != nil {
		uids, err := state.ClusterState.GetUidListByLabel(*host.Selector)
		if err != nil {
			ctrl.Log.WithName("Fill address").Error(err, "")
		}
		for _, uid := range uids {
			address, e := state.ClusterState.GetAddressByUid(types2.UID(uid))
			if e != nil {
				ctrl.Log.WithName("Fill address").Error(e, "")
			}
			addressList = append(addressList, address)
		}
		return addressList
	}
	return addressList
}
