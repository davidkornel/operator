package state

import (
	"context"
	"errors"
	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
	Pods           []corev1.Pod
	Vsvcs          []l7mpiov1.VirtualService
	Config         *rest.Config
	ClientSet      *kubernetes.Clientset
	VsvcRestClient *rest.RESTClient
}

var (
	ClusterState = NewKubernetesCluster()
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

func NewKubernetesCluster() KubernetesCluster {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	crdConfig := *config
	crdConfig.ContentConfig.GroupVersion = &l7mpiov1.GroupVersion
	crdConfig.APIPath = "/apis"
	crdConfig.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	crdConfig.UserAgent = rest.DefaultKubernetesUserAgent()
	vsvcRestClient, err := rest.UnversionedRESTClientFor(&crdConfig)
	if err != nil {
		panic(err)
	}
	// creates the ClientSet
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return KubernetesCluster{
		Pods:           []corev1.Pod{},
		Vsvcs:          []l7mpiov1.VirtualService{},
		Config:         config,
		ClientSet:      clientSet,
		VsvcRestClient: vsvcRestClient,
	}
}

func (s *KubernetesCluster) GetAddressByUid(uid types.UID) (string, error) {
	for _, p := range s.Pods {
		if p.UID == uid {
			return p.Status.PodIP, nil
		}
	}
	return "", errors.New("there is no pod with the given uid")
}

func (s *KubernetesCluster) GetUidListByLabel(logger logr.Logger, m map[string]string, notTerminating bool) ([]string, error) {
	stringList := make([]string, 0)
	podsFromK8sApi, err := GetPodList(logger, nil)
	if err != nil {
		panic(err.Error())
	}
	for k, v := range m {
		for _, p := range podsFromK8sApi.Items {
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

func (s *KubernetesCluster) RemoveElementFromSlice(logger logr.Logger, element l7mpiov1.VirtualService) {
	for i, e := range ClusterState.Vsvcs {
		if e.Name == element.Name {
			s.Vsvcs = append(s.Vsvcs[:i], s.Vsvcs[i+1:]...)
		}
	}
	logger.Info("Could not remove virtualservice from stored list, may require further investigation or it may have been a leftover from a previous deployment")
}

func GetVirtualServiceList(logger logr.Logger, vsvcs *l7mpiov1.VirtualServiceList) error {
	err := ClusterState.VsvcRestClient.Get().Resource("virtualservices").Do(context.TODO()).Into(vsvcs)
	if err != nil {
		logger.Error(err, "Error while asking K8s API for a VSVC list")
		return err
	} else {
		logger.Info("Successfully got the list of VirtualServices from the K8s API", "number of vsvcs", len(vsvcs.Items))
		return nil
	}
}

func GetPodList(logger logr.Logger, selector *metav1.LabelSelector) (*v1.PodList, error) {
	pods := &v1.PodList{}
	var err error
	if selector == nil {
		pods, err = ClusterState.ClientSet.CoreV1().Pods("").
			List(context.TODO(), metav1.ListOptions{})
	} else {
		pods, err = ClusterState.ClientSet.CoreV1().Pods("").
			List(context.TODO(), metav1.ListOptions{LabelSelector: metav1.FormatLabelSelector(selector)})
	}
	if err != nil {
		logger.Error(err, "Error while asking k8s API for a POD list")
		return nil, err
	} else {
		logger.Info("Successfully got the list of VirtualServices from the K8s API", "", len(pods.Items))
		return pods, nil
	}
}
