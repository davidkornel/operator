// Package controlplane Copyright 2020 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
package controlplane

import (
	"fmt"
	ds "github.com/davidkornel/operator/controlplane/discoveryservices"
	//"context"
	//"fmt"
	//clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	//endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"net"
	ctrl "sigs.k8s.io/controller-runtime"
	//testv3 "github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"google.golang.org/grpc"
)

const (
	grpcMaxConcurrentStreams = 1000000
)

var (
	cache cachev3.SnapshotCache
)

func registerServer(grpcServer *grpc.Server, srv listenerservice.ListenerDiscoveryServiceServer) {
	// register services
	//	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	//	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, srv)
}

// RunServer starts an xDS server at the given listenerPort.
func RunServer(port uint, l *Logger) {
	logger := ctrl.Log.WithName("management server")
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems.
	// Create a cache
	//cache = cachev3.NewSnapshotCache(false, cachev3.IDHash{}, l)
	//
	//ctx := context.Background()
	//cb := &testv3.Callbacks{Debug: false}
	//srv3 := serverv3.NewServer(ctx, cache, cb)
	//
	//var grpcOptions []grpc.ServerOption
	//grpcOptions = append(
	//	grpcOptions,
	//	grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
	//)
	//grpcServer := grpc.NewServer(grpcOptions...)
	//
	//lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	//if err != nil {
	//	logger.Error(err, "Managements server error: %s")
	//}
	//
	//logger.Info("management server listening on ", "port:", port)
	//
	//if err = grpcServer.Serve(lis); err != nil {
	//	logger.Error(err, "Error happened while serving the gRPC server")
	//}
	logger.Info("server.go here")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logger.Error(err, "Managements server error: %s")
		fmt.Println("tcp listener error ")
	} else {
		logger.Info("TCP server for the management server successfully started at ", "port:", port)
	}

	var grpcOptions []grpc.ServerOption
	grpcOptions = append(
		grpcOptions,
		grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams),
	)
	grpcServer := grpc.NewServer(grpcOptions...)
	registerServer(grpcServer, ds.NewServer())
	//pb.RegisterListenerDiscoveryServiceServer(grpcServer, ds.NewServer())
	e := grpcServer.Serve(lis)
	if e != nil {
		fmt.Println("Error happened while serving the gRPC server: ", e)
		logger.Error(e, "Error happened while serving the gRPC server")
	} else {
		logger.Info("Management server running at", "port:", port)
	}
}
