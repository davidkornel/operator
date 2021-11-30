package discoveryservices

import (
	"context"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	"github.com/davidkornel/operator/state"
	pb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	udp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"
	envoyservicediscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	"github.com/golang/protobuf/ptypes/any"
	"google.golang.org/protobuf/types/known/anypb"
	"io"
	ctrl "sigs.k8s.io/controller-runtime"
)

type listenerDiscoveryService struct {
	pb.UnimplementedListenerDiscoveryServiceServer
}

func NewLdsServer() *listenerDiscoveryService {
	s := &listenerDiscoveryService{
		UnimplementedListenerDiscoveryServiceServer: pb.UnimplementedListenerDiscoveryServiceServer{},
	}
	logger := ctrl.Log.WithName("LDS server")
	logger.Info("init")
	return s
}

func (s *listenerDiscoveryService) StreamListeners(_ listenerservice.ListenerDiscoveryService_StreamListenersServer) error {
	panic("implement me")
}

func (s *listenerDiscoveryService) FetchListeners(_ context.Context, _ *envoyservicediscoveryv3.DiscoveryRequest) (*envoyservicediscoveryv3.DiscoveryResponse, error) {
	panic("implement me")
}

func (s *listenerDiscoveryService) DeltaListeners(server listenerservice.ListenerDiscoveryService_DeltaListenersServer) error {
	logger := ctrl.Log.WithName("LDS")
	initialized := false
	for {
		//ddr delta discovery request
		ddr, err := server.Recv()
		if err == io.EOF {
			logger.Info("EOF")
			return nil
		}
		if err != nil {
			logger.Error(err, "ERROR")
			return err
		}
		logger.Info("Request:", "node id: ", ddr.Node.Id)
		if _, ok := state.LdsChannels[ddr.Node.Id]; !ok {
			state.LdsChannels[ddr.Node.Id] = make(chan state.SignalMessageOnLdsChannels)
			//logger.Info("Channel has been added for", "uid", ddr.Node.Id)
		}
		if !initialized {
			deltaDiscoveryResponse, initMsg := initLDSConnection(ddr.Node.Id)
			if initMsg != nil {
				logger.Info(*initMsg)
				initialized = true
			} else {
				sendErr := server.Send(deltaDiscoveryResponse)
				if sendErr != nil {
					logger.Error(sendErr, "Error occurred while SENDING listener configuration to envoy")
					return sendErr
				} else {
					logger.Info("Initialization was successful for", "node", ddr.Node.Id)
					initialized = true
				}
			}
		}
		ldsMessage, isOpen := <-state.LdsChannels[ddr.Node.Id]
		if !isOpen {
			//TODO Close connection from serverside
			logger.Info("gRPC connection should have ended here")
			break
		}
		switch ldsMessage.Verb {

		case state.Add:
			listeners := ldsMessage.Resources
			//logger.Info("listeners", "l", listeners)
			err = server.Send(createListenerDeltaDiscoveryResponse(listeners, nil))
			if err != nil {
				logger.Error(err, "Error occurred while SENDING listener configuration to envoy")
				return err
			}

		case state.Delete:
			listenersToBeDeleted := make([]string, 0)
			for _, l := range ldsMessage.Resources {
				listenersToBeDeleted = append(listenersToBeDeleted, l.Name)
			}
			err = server.Send(createListenerDeltaDiscoveryResponse(nil, listenersToBeDeleted))
			if err != nil {
				logger.Error(err, "Error occurred while REMOVING listener configuration to envoy")
				return err
			}

		case state.Change:
			panic("change not implemented")
			//TODO implement
		}
	}
	return nil
}

func initLDSConnection(uid string) (*envoyservicediscoveryv3.DeltaDiscoveryResponse, *string) {
	podName := ""
	for _, p := range state.ClusterState.Pods {
		if string(p.UID) == uid {
			podName = p.Name
			for k, v := range p.Labels {
				for _, vsvc := range state.ClusterState.Vsvcs {
					if vsvc.Spec.Selector[k] == v {
						listeners := CreateEnvoyListenerConfigFromVsvcSpec(vsvc.Spec)
						return createListenerDeltaDiscoveryResponse(listeners, nil), nil
					}
				}
			}
		}
	}
	msg := "There is no VSVC in the kubernetes cluster that should be applied for this pod " + podName
	return nil, &msg
}

func createListenerDeltaDiscoveryResponse(listenersToBeAdded []*listener.Listener, listenersToBeDeleted []string) *envoyservicediscoveryv3.DeltaDiscoveryResponse {
	var resources []*envoyservicediscoveryv3.Resource
	for _, l := range listenersToBeAdded {
		r := &envoyservicediscoveryv3.Resource{
			Name:     l.Name,
			Version:  "1",
			Resource: ConvertListenersToAny(l),
		}
		resources = append(resources, r)
	}

	response := envoyservicediscoveryv3.DeltaDiscoveryResponse{
		SystemVersionInfo: "Testversion",
		Resources:         resources,
		TypeUrl:           "type.googleapis.com/envoy.config.listener.v3.Listener",
		RemovedResources:  listenersToBeDeleted,
		Nonce:             "listener",
	}

	return &response
}

func ConvertListenersToAny(l *listener.Listener) *any.Any {
	listenerAny, _ := anypb.New(l)
	return listenerAny
}

func CreateEnvoyListenerConfigFromVsvcSpec(spec l7mpiov1.VirtualServiceSpec) []*listener.Listener {
	var listeners []*listener.Listener
	// TODO handle tcp listener as well
	for _, l := range spec.Listeners {
		udpFilter := &udp.UdpProxyConfig{
			StatPrefix: l.Name,
			RouteSpecifier: &udp.UdpProxyConfig_Cluster{
				Cluster: l.Udp.Cluster.Name,
			},
			HashPolicies: []*udp.UdpProxyConfig_HashPolicy{{
				PolicySpecifier: &udp.UdpProxyConfig_HashPolicy_Key{
					Key: l.Udp.Cluster.HashKey,
				},
			},
			},
		}

		pbst, err := anypb.New(udpFilter)
		if err != nil {
			panic(err)
		}

		list := &listener.Listener{
			Name:      l.Name,
			ReusePort: true,
			Address: &core.Address{
				Address: &core.Address_SocketAddress{
					SocketAddress: &core.SocketAddress{
						Protocol: core.SocketAddress_UDP,
						Address:  "0.0.0.0",
						PortSpecifier: &core.SocketAddress_PortValue{
							PortValue: l.Udp.Port,
						},
					},
				},
			},
			ListenerFilters: []*listener.ListenerFilter{{
				Name: "envoy.filters.udp_listener.udp_proxy",
				ConfigType: &listener.ListenerFilter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}
		listeners = append(listeners, list)
	}
	return listeners
}
