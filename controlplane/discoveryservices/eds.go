package discoveryservices

import (
	"context"
	"errors"
	"fmt"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	"github.com/davidkornel/operator/state"
	pb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoyservicediscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	"github.com/go-logr/logr"
	"github.com/golang/protobuf/ptypes/any"
	"google.golang.org/protobuf/types/known/anypb"
	"io"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Endpoint struct {
	Name    string
	Address string
	Ep      l7mpiov1.Endpoint
	Port    uint32
}

type endpointDiscoveryService struct {
	pb.UnimplementedEndpointDiscoveryServiceServer
}

func NewEdsServer() *endpointDiscoveryService {
	s := &endpointDiscoveryService{
		UnimplementedEndpointDiscoveryServiceServer: pb.UnimplementedEndpointDiscoveryServiceServer{},
	}
	logger := ctrl.Log.WithName("EDS server")
	logger.Info("init")
	return s
}

func (e endpointDiscoveryService) StreamEndpoints(_ endpointservice.EndpointDiscoveryService_StreamEndpointsServer) error {
	panic("implement me")
}

func (e endpointDiscoveryService) FetchEndpoints(_ context.Context, _ *envoyservicediscoveryv3.DiscoveryRequest) (*envoyservicediscoveryv3.DiscoveryResponse, error) {
	panic("implement me")
}

func (e endpointDiscoveryService) DeltaEndpoints(server endpointservice.EndpointDiscoveryService_DeltaEndpointsServer) error {
	logger := ctrl.Log.WithName("EDS")
	ownerCluster := ""
	for {
		//ddr delta discovery request
		ddr, err := server.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		logger.Info("Request:", "Node-id: ", ddr.Node.Id, "rns", ddr.ResourceNamesSubscribe)
		if ownerCluster == "" {
			//init message
			ownerCluster = ddr.ResourceNamesSubscribe[0]
			for _, rns := range ddr.ResourceNamesSubscribe {
				deltaDiscoveryResponse, err := initEdsConnection(logger, rns)
				if err != nil {
					logger.Error(err, "Error occurred while CREATING delta discovery response")
					return nil
				} else {
					if deltaDiscoveryResponse == nil {
						logger.Info("There is no endpoint match the label selector currently for",
							"cluster name", ownerCluster,
							"label selector", state.ConnectedEdsClients[ownerCluster].Endpoint.Host.Selector,
						)
					} else {
						err = server.Send(deltaDiscoveryResponse)
						if err != nil {
							logger.Error(err, "Error occurred while SENDING listener configuration to envoy")
						} else {
							logger.Info("Initial update was successful for cluster",
								"cluster's name", ownerCluster,
								"number of endpoints added", len(deltaDiscoveryResponse.Resources))
						}
					}
				}
			}
		}
		edsMessage, isOpen := <-state.ConnectedEdsClients[ownerCluster].Channel
		if !isOpen {
			//TODO Close connection from serverside
			logger.Info("gRPC connection should have ended here")
			delete(state.ConnectedEdsClients, ownerCluster)
			return nil
		}
		switch edsMessage.Verb {

		case state.Add:
			err = server.Send(createEndpointDeltaDiscoveryResponse(logger, edsMessage.Resource))
			if err != nil {
				logger.Error(err, "Error occurred while SENDING endpoint configuration to envoy")
				return err
			} else {
				logger.Info("EDS response", "number of endpoints", len(edsMessage.Resource.Endpoints), "endpoints", edsMessage.Resource.Endpoints)
			}
		}

	}
}

func initEdsConnection(logger logr.Logger, rns string) (*envoyservicediscoveryv3.DeltaDiscoveryResponse, error) {

	if state.ConnectedEdsClients[rns] != nil {

		selector := *state.ConnectedEdsClients[rns].Endpoint.Host.Selector
		ep := state.ConnectedEdsClients[rns].Endpoint
		port := state.ConnectedEdsClients[rns].Endpoint.Port
		endpointsToAdd := make([]Endpoint, 0)

		for _, p := range state.ClusterState.Pods {
			//logger.Info("iter thru pod name", "", p.Name)
			//logger.Info("pod label", "", p.Labels)
			//logger.Info("vsvc selector", "", selector)
			for k, v := range p.Labels {
				if selector[k] == v {
					if p.Status.PodIP != "" {
						//logger.Info("creating endpoints for ", "cluster", rns, "pod", p.Name)
						endpointsToAdd = append(endpointsToAdd, Endpoint{
							Name:    rns + "-" + p.Name,
							Address: p.Status.PodIP,
							Port:    port,
							Ep:      ep,
						})
					}
				}
			}
		}
		if len(endpointsToAdd) > 0 {
			claToAdd := CreateEdsEndpoint(logger, endpointsToAdd, rns)
			return createEndpointDeltaDiscoveryResponse(logger, claToAdd), nil
		} else {
			// return empty endpoint list in order to prevent envoy from stalling in the EDS init phase
			emptyEndpointSlice := make([]Endpoint, 0)
			return createEndpointDeltaDiscoveryResponse(logger, CreateEdsEndpoint(logger, emptyEndpointSlice, rns)), nil
		}
	}
	return nil, errors.New(fmt.Sprintf("ConnectedEdsClients does not have key %s", rns))
}

func CreateEdsEndpoint(_ logr.Logger, endpoints []Endpoint, clusterName string) *endpointv3.ClusterLoadAssignment {
	var lbEndpoints []*endpointv3.LbEndpoint

	loadAssignment := &endpointv3.ClusterLoadAssignment{
		ClusterName: clusterName,
	}

	for _, e := range endpoints {
		newEndpoint := &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_UDP,
								Address:  e.Address,
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: e.Port,
								},
							},
						},
					},
					HealthCheckConfig: createEndpointHealthCheckConfig(e.Ep),
				},
			},
			Metadata: &core.Metadata{FilterMetadata: createLoadBalancerStruct(e.Address)},
		}
		//logger.Info("newendpoint", "", newEndpoint)
		lbEndpoints = append(lbEndpoints, newEndpoint)

	}
	localityLbEndpoints := []*endpointv3.LocalityLbEndpoints{{LbEndpoints: lbEndpoints}}
	loadAssignment.Endpoints = localityLbEndpoints
	//logger.Info("loadassignement", "", loadAssignment)
	//logger.Info("endpoints.len", "", len(endpoints))
	//logger.Info("loadassignement.endpoints.len", "", len(loadAssignment.Endpoints))
	return loadAssignment
}

func createEndpointDeltaDiscoveryResponse(_ logr.Logger, claToAdd *endpointv3.ClusterLoadAssignment) *envoyservicediscoveryv3.DeltaDiscoveryResponse {
	var resources []*envoyservicediscoveryv3.Resource
	r := &envoyservicediscoveryv3.Resource{
		Name:     claToAdd.ClusterName,
		Version:  "1",
		Resource: ConvertClusterLoadAssignmentToAny(claToAdd),
	}
	resources = append(resources, r)

	response := envoyservicediscoveryv3.DeltaDiscoveryResponse{
		SystemVersionInfo: "Testversion",
		Resources:         resources,
		TypeUrl:           "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment",
		RemovedResources:  nil, //EDS can't handle updates on per endpoint basis, all the endpoints must be sent all the time
		Nonce:             "listener",
	}
	return &response
}

func ConvertClusterLoadAssignmentToAny(cla *endpointv3.ClusterLoadAssignment) *any.Any {
	claAny, _ := anypb.New(cla)
	return claAny
}
