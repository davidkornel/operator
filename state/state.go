package state

import (
	"errors"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// KubernetesCluster State of the k8s cluster
type KubernetesCluster struct {
	Pods  []corev1.Pod
	Vsvcs []l7mpiov1.VirtualServiceSpec
}

var (
	ClusterState = KubernetesCluster{}
	VsvcChannel  = make(chan l7mpiov1.VirtualServiceSpec)
	// LdsChannels Keys are the 'node.id's from the connected envoy instance
	//which is the same as the pod UID
	//where the container of the envoy instance is located in
	//Used for signaling to the gRPC server that there is config available for configuration
	LdsChannels = make(map[string]chan []*listener.Listener)
	// CdsChannels Keys are the 'node.id's from the connected envoy instance
	//which is the same as the pod UID
	//where the container of the envoy instance is located in
	//Used for signaling to the gRPC server that there is config available for configuration
	CdsChannels = make(map[string]chan []*cluster.Cluster)
)

func (s *KubernetesCluster) GetAddressByUid(uid types.UID) (string, error) {
	for _, p := range s.Pods {
		if p.UID == uid {
			return p.Status.PodIP, nil
		}
	}
	return "", errors.New("there is no pod with the given uid")
}

func (s *KubernetesCluster) GetUidByLabel(m map[string]string) (string, error) {
	for k, v := range m {
		for _, p := range s.Pods {
			if p.Labels[k] == v {
				return string(p.UID), nil
			}
		}
	}
	return "", errors.New("there is no pod with the given label")
}
