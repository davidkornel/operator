package cluster

import (
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type State struct {
	Pods  map[string]corev1.Pod
	Vsvcs map[string]l7mpiov1.VirtualService
}

var StateOfCluster = State{}

func (s *State) GetAddressByUid(uid types.UID) string {
	uidAsString := string(uid)
	return s.Pods[uidAsString].Status.PodIP
}
