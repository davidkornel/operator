package discoveryservices

import (
	"context"
	"errors"
	"fmt"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	"github.com/davidkornel/operator/state"
	pb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoyservicediscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/go-logr/logr"
	"github.com/golang/protobuf/ptypes/any"
	_struct "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

type clusterDiscoveryService struct {
	pb.UnimplementedClusterDiscoveryServiceServer
}

func NewCdsServer() *clusterDiscoveryService {
	s := &clusterDiscoveryService{
		UnimplementedClusterDiscoveryServiceServer: pb.UnimplementedClusterDiscoveryServiceServer{},
	}
	logger := ctrl.Log.WithName("CDS server")
	logger.Info("init")
	return s
}

func (c clusterDiscoveryService) StreamClusters(_ clusterservice.ClusterDiscoveryService_StreamClustersServer) error {
	panic("implement me")
}

func (c clusterDiscoveryService) FetchClusters(_ context.Context, _ *envoyservicediscoveryv3.DiscoveryRequest) (*envoyservicediscoveryv3.DiscoveryResponse, error) {
	panic("implement me")
}

func (c clusterDiscoveryService) DeltaClusters(server clusterservice.ClusterDiscoveryService_DeltaClustersServer) error {
	logger := ctrl.Log.WithName("CDS")
	initialized := false
	podName := ""
	for {
		//ddr delta discovery request
		ddr, err := server.Recv()
		if err == io.EOF {
			logger.Info("EOF")
			return nil
		}
		if err != nil {
			//TODO fix this but it might be a context canceled error
			logger.Error(err, "Error occurred while receiving DDR")
			break
		}
		logger.Info("Request (if empty string then it's the first request)", "pod", podName, "uid", ddr.Node.Id)
		if _, ok := state.CdsChannels[ddr.Node.Id]; !ok {
			state.CdsChannels[ddr.Node.Id] = make(chan state.SignalMessageOnCdsChannels)
			//logger.Info("Channel has been added for", "uid", ddr.Node.Id)
		}
		if !initialized {
			deltaDiscoveryResponse, cName, initMsg := initCDSConnection(logger, ddr.Node.Id)
			podName = *cName
			if initMsg != nil {
				logger.Info(*initMsg)
				initialized = true
			} else {
				sendErr := server.Send(deltaDiscoveryResponse)
				if sendErr != nil {
					logger.Info("Error occurred while SENDING cluster configuration to envoy", "error", sendErr.Error())
					break
				} else {
					logger.Info("Initialization was successful for", "node", ddr.Node.Id)
					initialized = true
				}
			}
		}
		cdsMessage, isOpen := <-state.CdsChannels[ddr.Node.Id]
		if !isOpen {
			logger.Info("Closing gRPC connection from server side")
			break
		}
		switch cdsMessage.Verb {

		case state.Add:
			clusters := cdsMessage.Resources
			//logger.Info("clusters", "c", clusters)
			err = server.Send(createClusterDeltaDiscoveryResponse(clusters, nil))
			logger.Info("Clusters to be added", "num", len(clusters))
			if err != nil {
				logger.Info("Error occurred while SENDING cluster configuration to envoy", "error", err.Error())
				break
			}

		case state.Delete:
			clustersToBeDeleted := make([]string, 0)
			for _, c := range cdsMessage.Resources {
				clustersToBeDeleted = append(clustersToBeDeleted, c.Name)
			}
			logger.Info("Clusters to be removed", "clusters", clustersToBeDeleted)
			err = server.Send(createClusterDeltaDiscoveryResponse(nil, clustersToBeDeleted))
			if err != nil {
				logger.Info("Error occurred while REMOVING cluster configuration from envoy", "error", err.Error())
				break
			}

		case state.Change:
			panic("change not implemented")
			//TODO implement
		}
	}
	return nil
}

func contains(logger logr.Logger, e types.UID) *v1.Pod {
	for _, a := range state.ClusterState.Pods {
		//logger.Info("contains", "got uid", string(e), "pods uid", string(a.UID), "podname", a.Name)
		if a.UID == e {
			logger.Info("contains MATCH", "pod", a.Name)
			return &a
		}
	}
	return nil
}

func initCDSConnection(logger logr.Logger, uid string) (*envoyservicediscoveryv3.DeltaDiscoveryResponse, *string, *string) {
	podName := ""
	var clusters []*cluster.Cluster
	deadline := time.Now().Add(25 * time.Second)
	UID := types.UID(uid)
	logger.Info("initCDS", "uid", uid)
	for {
		//TODO REFACTOR the code below, should look like initLDS...
		if p := contains(logger, UID); p != nil {
			podName = p.Name
			vsvcs := l7mpiov1.VirtualServiceList{}
			err := state.GetVirtualServiceList(logger, &vsvcs)
			if err != nil {
				msg := "Error happened while requesting VSVC list from the Kubernetes API"
				return nil, &podName, &msg
			}
			for i, vsvc := range vsvcs.Items {
				for k, v := range p.Labels {
					if vsvc.Spec.Selector[k] == v {
						clusters = append(clusters, CreateEnvoyClusterConfigFromVsvcSpec(vsvc.Spec, uid)...)
					}
				}
				if i+1 == len(vsvcs.Items) {
					if len(clusters) > 0 {
						return createClusterDeltaDiscoveryResponse(clusters, nil), &podName, nil
					} else {
						msg := "There is no VSVC in the kubernetes cluster that matches any of labels of this pod " + podName
						//TODO handle return better
						return nil, &podName, &msg
					}
				}
			}
			msg := "There is no VSVC in the kubernetes cluster that should be applied for this pod " + podName
			//TODO handle return better
			return nil, &podName, &msg
		} else {
			time.Sleep(500 * time.Millisecond)
		}
		if time.Now().After(deadline) {
			msg := "Pod not appeared to be ready for 25 seconds"
			logger.Error(errors.New("deadline passed"), msg)
			return nil, &podName, &msg
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

func CreateEnvoyClusterConfigFromVsvcSpec(spec l7mpiov1.VirtualServiceSpec, uid string) []*cluster.Cluster {
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
			LbPolicy:                  cluster.Cluster_MAGLEV,
			IgnoreHealthOnHostRemoval: true,
		}
		if l.Udp.Cluster.HealthCheck != nil {
			if l.Udp.Cluster.HealthCheck.Protocol == "TCP" {
				c.HealthChecks = []*core.HealthCheck{{
					Timeout:            durationpb.New(100 * time.Millisecond),
					Interval:           durationpb.New(time.Duration(l.Udp.Cluster.HealthCheck.Interval) * time.Millisecond),
					UnhealthyThreshold: &wrappers.UInt32Value{Value: 1},
					HealthyThreshold:   &wrappers.UInt32Value{Value: 1},
					HealthChecker: &core.HealthCheck_HttpHealthCheck_{
						HttpHealthCheck: &core.HealthCheck_HttpHealthCheck{
							Path: "/healthcheck",
						},
					},
					//HealthChecker: &core.HealthCheck_TcpHealthCheck_{
					//	TcpHealthCheck: &core.HealthCheck_TcpHealthCheck{
					//		Send: &core.HealthCheck_Payload{
					//			Payload: &core.HealthCheck_Payload_Text{
					//				Text: "000000FF",
					//			},
					//		},
					//		Receive: []*core.HealthCheck_Payload{{
					//			Payload: &core.HealthCheck_Payload_Text{
					//				Text: "000000FF",
					//			},
					//		}},
					//	},
					//},
					NoTrafficInterval: durationpb.New(time.Duration(l.Udp.Cluster.HealthCheck.Interval) * time.Millisecond),
				}}
			}
		}

		switch l.Udp.Cluster.ServiceDiscovery {
		case "strictdns":
			createStrictDnsConfig(c, l)
		case "eds":
			//TODO handle eds
			e := l.Udp.Cluster.Endpoints[0]
			createEdsConfig(c, e, uid)
		}

		clusters = append(clusters, c)
	}
	return clusters
}

func createEdsConfig(c *cluster.Cluster, endpoint l7mpiov1.Endpoint, uid string) {
	if _, ok := state.ConnectedEdsClients[c.Name]; !ok {
		state.ConnectedEdsClients[c.Name] = &state.EdsClient{
			Endpoint:    endpoint,
			Channel:     make(chan state.SignalMessageOnEdsChannels),
			UID:         uid,
			PodSelector: *endpoint.Host.Selector,
		}
	}

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
					SetNodeOnFirstMessageOnly: true,
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
	c.LoadAssignment = createStrictDnsEndpoints(l.Udp.Cluster)
}

func createStrictDnsEndpoints(c l7mpiov1.Cluster) *endpointv3.ClusterLoadAssignment {

	loadAssignment := &endpointv3.ClusterLoadAssignment{
		ClusterName: c.Name,
	}
	lbEndpoints := make([]*endpointv3.LbEndpoint, 0)
	for _, e := range c.Endpoints {
		for _, a := range fillAddress(e.Host) {
			newEndpoint := &endpointv3.LbEndpoint{
				HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
					Endpoint: &endpointv3.Endpoint{
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
				Metadata: &core.Metadata{FilterMetadata: createLoadBalancerStruct(a)},
			}
			lbEndpoints = append(lbEndpoints, newEndpoint)
		}
	}
	localityLbEndpoints := []*endpointv3.LocalityLbEndpoints{{LbEndpoints: lbEndpoints}}
	loadAssignment.Endpoints = localityLbEndpoints
	return loadAssignment
}

func createEndpointHealthCheckConfig(e l7mpiov1.Endpoint) *endpointv3.Endpoint_HealthCheckConfig {
	if e.HealthCheckPort != nil {
		hc := &endpointv3.Endpoint_HealthCheckConfig{
			PortValue: *e.HealthCheckPort,
		}
		return hc
	}
	return nil
}

func createLoadBalancerStruct(value string) map[string]*structpb.Struct {
	m, err := structpb.NewStruct(map[string]interface{}{
		"hash_key": fmt.Sprintf("%s", value),
	})
	if err != nil {
		ctrl.Log.WithName("Endpoint").Error(err, "Error occurred while running stuctpb.NewStruct")
	}
	return map[string]*_struct.Struct{"envoy.lb": m}
}

func fillAddress(host l7mpiov1.Host) []string {
	logger := ctrl.Log.WithName("Fill address")
	addressList := make([]string, 0)
	if host.Address != nil {
		addressList = append(addressList, *host.Address)
		return addressList
	}
	if host.Selector != nil {
		uids, err := state.ClusterState.GetUidListByLabel(logger, *host.Selector, false)
		if err != nil {
			logger.Error(err, "Getting UID list failed")
		}
		for _, uid := range uids {
			address, e := state.ClusterState.GetAddressByUid(types.UID(uid))
			if e != nil {
				logger.Error(err, "Getting address failed")
			}
			addressList = append(addressList, address)
		}
		return addressList
	}
	return addressList
}
