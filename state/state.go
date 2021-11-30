package state

import (
	"errors"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type Verb int

const (
	Add Verb = iota
	Delete
	Change
)

type SignalMessageOnLdsChannels struct {
	Verb      Verb
	Resources []*listener.Listener
}
type SignalMessageOnCdsChannels struct {
	Verb      Verb
	Resources []*cluster.Cluster
}
type SignalMessageOnEdsChannels struct {
	Verb     Verb
	Resource *endpoint.ClusterLoadAssignment
}
type EdsClient struct {
	Endpoint    l7mpiov1.Endpoint
	Channel     chan SignalMessageOnEdsChannels
	UID         string
	PodSelector map[string]string
}
type SignalMessageOnPodChannel struct {
	Verb Verb
	Pod  *corev1.Pod
}

// KubernetesCluster State of the k8s cluster
type KubernetesCluster struct {
	Pods  []corev1.Pod
	Vsvcs []l7mpiov1.VirtualService
}

var (
	ClusterState = KubernetesCluster{}
	VsvcChannel  = make(chan l7mpiov1.VirtualServiceSpec)
	PodChannel   = make(chan SignalMessageOnPodChannel)
	// LdsChannels Keys are the 'node.id's from the connected envoy instance
	//which is the same as the pod UID
	//where the container of the envoy instance is located in
	//Used for signaling to the gRPC server that there is config available for configuration
	LdsChannels = make(map[string]chan SignalMessageOnLdsChannels)
	// CdsChannels Keys are the 'node.id's from the connected envoy instance
	//which is the same as the pod UID
	//where the container of the envoy instance is located in
	//Used for signaling to the gRPC server that there is config available for configuration
	CdsChannels = make(map[string]chan SignalMessageOnCdsChannels)
	// ConnectedEdsClients Keys are the owner cluster's name from the connected envoy instance
	//which is the same as the pod UID
	//where the container of the envoy instance is located in
	//Used for signaling to the gRPC server that there is config available for configuration
	ConnectedEdsClients = make(map[string]*EdsClient)
)

func (s *KubernetesCluster) GetAddressByUid(uid types.UID) (string, error) {
	for _, p := range s.Pods {
		if p.UID == uid {
			return p.Status.PodIP, nil
		}
	}
	return "", errors.New("there is no pod with the given uid")
}

func (s *KubernetesCluster) GetUidListByLabel(m map[string]string) ([]string, error) {
	stringList := make([]string, 0)
	for k, v := range m {
		for _, p := range s.Pods {
			if p.Labels[k] == v {
				stringList = append(stringList, string(p.UID))
			}
		}
	}
	if len(stringList) == 0 {
		//TODO return not an error cuz there might be situations where there isn't a single pod with the a given label
		return stringList, errors.New("there is no pod with the given label")
	}
	return stringList, nil
}

func (s *KubernetesCluster) RemoveElementFromSlice(element l7mpiov1.VirtualService) error {
	for i, e := range ClusterState.Vsvcs {
		if e.Name == element.Name {
			s.Vsvcs = append(s.Vsvcs[:i], s.Vsvcs[i+1:]...)
			return nil
		}
	}
	return errors.New("could not remove vsvc from slice")
}
