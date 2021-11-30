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

package controllers

import (
	"context"
	"github.com/davidkornel/operator/state"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	labelKeys = "app"
)

var (
	//The pod labelValues that we are interested in
	labelValues = []string{"envoy-ingress", "worker"}
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=pods/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("POD_C")
	// your logic here
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since we can get them on deleted requests.
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Pod")
		return ctrl.Result{}, err
	}
	isPodMarkedToBeDeleted := pod.GetDeletionTimestamp() != nil
	if isPodMarkedToBeDeleted {
		//logger.Info("pod marked to be deleted")
		//TODO this is a huge mess but works, reduce loops and 'if's
		for _, a := range pod.Status.ContainerStatuses {
			//TODO with graceperiod set to 0 or 1 container won't reach the terminated state
			if a.Name == "envoy" /*&& a.State.Terminated != nil*/ {
				for i, p := range state.ClusterState.Pods {
					if p.UID == pod.UID {
						state.ClusterState.Pods = remove(state.ClusterState.Pods, i)

						delete(state.LdsChannels, string(pod.UID))
						delete(state.LdsChannels, string(pod.UID))
						go r.CloseChannels(logger, ctx, pod)
						logger.Info("Removed pod from Pods", "pod", pod.Name, "uid", pod.UID)
						logger.Info("Number of elements in Pods", "", len(state.ClusterState.Pods))
						state.PodChannel <- state.SignalMessageOnPodChannel{
							Verb: 1,
							Pod:  &pod,
						}
						return ctrl.Result{}, nil
					}
				}
			}
		}

		return ctrl.Result{}, nil
	}
	for _, label := range labelValues {
		labelIsPresent := pod.Labels[labelKeys] == label
		if labelIsPresent {
			//TODO should we wait till the pod is in phase running or not???
			if pod.Status.Phase == "Running" && pod.Status.PodIP != "" {
				for i, p := range state.ClusterState.Pods {
					if p.UID == pod.UID {
						state.ClusterState.Pods[i] = pod
						//logger.Info("pod changed in Pods:", "name: ", pod.Name, "pod.uid: ", pod.UID)
						return ctrl.Result{}, nil
					}
				}
				state.ClusterState.Pods = append(state.ClusterState.Pods, pod)
				state.PodChannel <- state.SignalMessageOnPodChannel{
					Verb: 0,
					Pod:  &pod,
				}

				logger.Info("Pod added to Pods:", "name: ", pod.Name, " pod.uid: ", pod.UID)
			}
		}
	}
	return ctrl.Result{}, nil
}

func (r *PodReconciler) CloseChannels(logger logr.Logger, ctx context.Context, pod corev1.Pod) {
	for {
		p := corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}, &p); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Pod not found, closing its channels")
				if _, ok := state.LdsChannels[string(pod.UID)]; ok {
					close(state.LdsChannels[string(pod.UID)])
				}
				if _, ok := state.CdsChannels[string(pod.UID)]; ok {
					close(state.CdsChannels[string(pod.UID)])
				}
				for _, e := range state.ConnectedEdsClients {
					if e.UID == string(pod.UID) {
						close(e.Channel)
						logger.Info("Closing eds channel")
					}
				}
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}

func remove(s []corev1.Pod, i int) []corev1.Pod {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}
