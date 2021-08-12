package discoveryservices

import (
	"context"
	pb "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyservicediscoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	"io"
	ctrl "sigs.k8s.io/controller-runtime"
)

type listenerDiscoveryService struct {
	pb.UnimplementedListenerDiscoveryServiceServer
	//TODO to understand what should I add here
}

func (s *listenerDiscoveryService) StreamListeners(server listenerservice.ListenerDiscoveryService_StreamListenersServer) error {
	panic("implement me")
}

func (s *listenerDiscoveryService) FetchListeners(ctx context.Context, request *envoyservicediscoveryv3.DiscoveryRequest) (*envoyservicediscoveryv3.DiscoveryResponse, error) {
	panic("implement me")
}

func (s *listenerDiscoveryService) DeltaListeners(server listenerservice.ListenerDiscoveryService_DeltaListenersServer) error {
	logger := ctrl.Log.WithName("LDS")
	logger.Info("LDS INIT")
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
	}
}

func NewServer() *listenerDiscoveryService {
	s := &listenerDiscoveryService{
		UnimplementedListenerDiscoveryServiceServer: pb.UnimplementedListenerDiscoveryServiceServer{},
	}
	logger := ctrl.Log.WithName("newserver")
	logger.Info("newserver init")
	return s
}
