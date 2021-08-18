/*
   Copyright 2021.

   Licensed under the Apache License, Version 2.0 (the "License");
                          you may not use this file except in compliance with the License.
                          You may obtain a copy of the License at

                          http://www.apache.org/licenses/LICENSE-2.0

                          Unless required by applicable law or agreed to in writing, software
                          distributed under the License is distributed on an "AS IS" BASIS,
                          WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
                          See the License for the specific language governing permissions and
                          limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VirtualServiceStatus defines the observed state of VirtualService
type VirtualServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type VirtualService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualServiceSpec   `json:"spec,omitempty"`
	Status VirtualServiceStatus `json:"status,omitempty"`
}

// VirtualServiceSpec defines the desired state of VirtualService
type VirtualServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of VirtualService. Edit virtualservice_types.go to remove/update

	//+kubebuilder:validation:Required
	Selector map[string]string `json:"selector"`
	//+kubebuilder:validation:Required
	Listeners []Listener `json:"listeners,omitempty"`
}

// Listener Listener configuration. Exactly one of UDP or TCP field should be used
type Listener struct {
	Name string `json:"name"`
	//+kubebuilder:validation:Optional
	Udp Udp `json:"udp,omitempty"`
	//+kubebuilder:validation:Optional
	Tcp Tcp `json:"tcp,omitempty"`
}

// HealthCheck Optional, if omitted all upstream hosts will be considered healthy all the time
type HealthCheck struct {
	//+kubebuilder:validation:Required
	Interval uint32 `json:"interval"`
	//+kubebuilder:validation:Enum=TCP
	Protocol string `json:"protocol"`
}

//+kubebuilder:validation:MaxProperties=1

type Host struct {
	//+kubebuilder:validation:Optional
	//Exactly one of address or selector field should be used
	//matchLabels is a map of {key,value} pairs.
	Selector *map[string]string `json:"selector,omitempty"`

	//+kubebuilder:validation:Optional
	//Exactly one of address or selector field should be used
	//		  Address of the upstream host.
	//		  Can be an exact address like 127.0.0.1
	//        Or a domain like my.domain
	Address *string `json:"address,omitempty"`
}

type Endpoint struct {
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	//+kubebuilder:validation:Optional
	//Exactly one of host or selector should be used
	//Address of the upstream host if no selector had been defined
	Host Host `json:"host"`

	//+kubebuilder:validation:Required
	//Upstream host's port
	Port uint32 `json:"port"`

	//+kubebuilder:validation:Optional
	// If HealthCheck configured in cluster definition this port will be used for health checking
	HealthCheckPort *uint32 `json:"health_check_port,omitempty"`
}

type Cluster struct {
	Name string `json:"name"`
	//+kubebuilder:validation:Enum=strictdns;eds
	ServiceDiscovery string `json:"service_discovery"`
	HashKey          string `json:"hash_key"`
	//+kubebuilder:validation:Optional
	HealthCheck *HealthCheck `json:"health_check,omitempty"`
	//+kubebuilder:validation:Required
	Endpoints []Endpoint `json:"endpoints"`
}

type Udp struct {
	//+kubebuilder:validation:Required
	//Port Listening port
	Port uint32 `json:"port"`
	//+kubebuilder:validation:Required
	Cluster Cluster `json:"cluster"`
}

type Tcp struct {
	Port uint32 `json:"port"`
}

//}
//+kubebuilder:object:root=true

// VirtualServiceList contains a list of VirtualService
type VirtualServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualService{}, &VirtualServiceList{})
}
