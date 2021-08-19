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
	"github.com/davidkornel/operator/state"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"

	//"context"
	//"fmt"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
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

func registerServer(grpcServer *grpc.Server, ldsServer listenerservice.ListenerDiscoveryServiceServer, cdsServer clusterservice.ClusterDiscoveryServiceServer) {
	// register services
	//	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, cdsServer)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, ldsServer)
}

// RunServer starts an xDS server at the given listenerPort.
func RunServer(port uint) {
	logger := ctrl.Log.WithName("management server")
	go VirtualServiceSpecHandler()

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
	registerServer(grpcServer, ds.NewLdsServer(), ds.NewCdsServer())
	e := grpcServer.Serve(lis)
	if e != nil {
		fmt.Println("Error happened while serving the gRPC server: ", e)
		logger.Error(e, "Error happened while serving the gRPC server")
	} else {
		logger.Info("Management server running at", "port:", port)
	}
}

func VirtualServiceSpecHandler() {
	logger := ctrl.Log.WithName("Vsvc spec handler")
	for {
		spec := <-state.VsvcChannel
		uids, err := state.ClusterState.GetUidListByLabel(spec.Selector)
		if err != nil {
			logger.Error(err, "Error occurred while trying to get uid by label selector")
		}
		for _, uid := range uids {
			logger.Info("", "uid:", uid)
			if _, ok := state.LdsChannels[uid]; !ok {
				state.LdsChannels[uid] = make(chan []*listener.Listener)
				logger.Info("LDS channel has been added for", "uid", uid)
			}
			if _, ok := state.CdsChannels[uid]; !ok {
				state.CdsChannels[uid] = make(chan []*cluster.Cluster)
				logger.Info("CDS channel has been added for", "uid", uid)
			}
			state.LdsChannels[uid] <- ds.CreateEnvoyListenerConfigFromVsvcSpec(spec)
			state.CdsChannels[uid] <- ds.CreateEnvoyClusterConfigFromVsvcSpec(spec)
			logger.Info("received", "spec:", spec)
		}

	}
}
