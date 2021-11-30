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
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"net"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	grpcMaxConcurrentStreams = 1000000
)

var (
	cache cachev3.SnapshotCache
)

func registerServer(
	grpcServer *grpc.Server, ldsServer listenerservice.ListenerDiscoveryServiceServer,
	cdsServer clusterservice.ClusterDiscoveryServiceServer,
	edsServer endpointservice.EndpointDiscoveryServiceServer) {

	// register services
	//	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, cdsServer)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, ldsServer)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, edsServer)
}

// RunServer starts an xDS server at the given listenerPort.
func RunServer(port uint) {
	logger := ctrl.Log.WithName("management server")

	go VirtualServiceSpecHandler()
	go PodHandler()

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
	registerServer(grpcServer, ds.NewLdsServer(), ds.NewCdsServer(), ds.NewEdsServer())
	e := grpcServer.Serve(lis)
	if e != nil {
		fmt.Println("Error happened while serving the gRPC server: ", e)
		logger.Error(e, "Error happened while serving the gRPC server")
	} else {
		logger.Info("Management server running at", "port:", port)
	}
}

func PodHandler() {
	logger := ctrl.Log.WithName("Pod handler")
	for {
		podMessage := <-state.PodChannel
		switch podMessage.Verb {
		//Both cases are handled the same because the new pod's identity
		case state.Add:
			logger.Info("ADD ACTION")
			Action(logger, podMessage.Pod.Labels)
		case state.Delete:
			logger.Info("DELETE ACTION")
			Action(logger, podMessage.Pod.Labels)
		}
	}
}

func Action(logger logr.Logger, labels map[string]string) {
	var epList []ds.Endpoint
	//If there is a connected eds client which waits for pods with the same label as the pod on the channel then
	// get the full list of pods with the same matching pod label
	for edsKey, edsValue := range state.ConnectedEdsClients {
		for labelKey, labelValue := range labels {
			//Process further only if the eds client's label selector matches one of the new pod's labels
			if (*edsValue.Endpoint.Host.Selector)[labelKey] == labelValue {
				for _, pod := range state.ClusterState.Pods {
					if pod.Labels[labelKey] == labelValue {
						ep := ds.Endpoint{
							Name:    edsKey + pod.Name,
							Address: pod.Status.PodIP,
							Ep:      state.ConnectedEdsClients[edsKey].Endpoint,
							Port:    state.ConnectedEdsClients[edsKey].Endpoint.Port,
						}
						//logger.Info("EDS label selector match with pod", "", labelKey, "", labelValue, "", ep)
						epList = append(epList, ep)
					}
				}
			}
		}
		if len(epList) > 0 {
			//During eds there is no Delete case but Add case. If an endpoint must be "removed" then the full list of endpoints
			// must be sent to the client leaving the "removed/deleted" endpoint out from this list.
			state.ConnectedEdsClients[edsKey].Channel <- state.SignalMessageOnEdsChannels{
				Verb:     0, //add
				Resource: ds.CreateEdsEndpoint(logger, epList, edsKey),
			}
			epList = []ds.Endpoint{}
		}
	}
}

func VirtualServiceSpecHandler() {
	logger := ctrl.Log.WithName("Vsvc spec handler")
	for {
		spec := <-state.VsvcChannel
		uids, err := state.ClusterState.GetUidListByLabel(spec.Selector)
		if err != nil {
			logger.Info("Non fatal error happened while trying to get uid by label selector")
		} else {
			for _, uid := range uids {
				logger.Info("", "uid:", uid)
				if _, ok := state.LdsChannels[uid]; !ok {
					state.LdsChannels[uid] = make(chan state.SignalMessageOnLdsChannels)
					logger.Info("LDS channel has been added for", "uid", uid)
				}
				if _, ok := state.CdsChannels[uid]; !ok {
					state.CdsChannels[uid] = make(chan state.SignalMessageOnCdsChannels)
					logger.Info("CDS channel has been added for", "uid", uid)
				}
				state.LdsChannels[uid] <- state.SignalMessageOnLdsChannels{
					Verb:      0, //Add
					Resources: ds.CreateEnvoyListenerConfigFromVsvcSpec(spec),
				}
				state.CdsChannels[uid] <- state.SignalMessageOnCdsChannels{
					Verb:      0,
					Resources: ds.CreateEnvoyClusterConfigFromVsvcSpec(spec, uid),
				}
			}
		}
		logger.Info("received spec with", "# of listeners:", len(spec.Listeners))

	}
}
