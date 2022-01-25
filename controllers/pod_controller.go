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
	logger := ctrl.Log.WithName("POD_CON")
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
	//if true then pod is marked to be deleted
	if pod.GetDeletionTimestamp() != nil {
		for _, p := range state.ClusterState.Pods {
			if p.UID == pod.UID {
				var ldsChan chan state.SignalMessageOnLdsChannels
				var cdsChan chan state.SignalMessageOnCdsChannels
				if _, ok := state.LdsChannels[string(pod.UID)]; ok {
					ldsChan = state.LdsChannels[string(pod.UID)]
					logger.Info("length of ldschannels before deletion", "len", len(state.LdsChannels))
					delete(state.LdsChannels, string(pod.UID))
					logger.Info("length of ldschannels after deletion", "len", len(state.LdsChannels))
					logger.Info("Deleted LDS channel for", "pod", pod.UID)
				}
				if _, ok := state.CdsChannels[string(pod.UID)]; ok {
					cdsChan = state.CdsChannels[string(pod.UID)]
					delete(state.CdsChannels, string(pod.UID))
					logger.Info("Deleted CDS channel for", "pod", pod.UID)
				}
				go r.CloseChannels(logger, ctx, pod, ldsChan, cdsChan)

				return ctrl.Result{}, nil
			}
		}
		return ctrl.Result{}, nil
	}
	for _, label := range labelValues {
		labelIsPresent := pod.Labels[labelKeys] == label
		if labelIsPresent {
			//TODO should we wait till the pod is in phase running or not???
			if isPodReadyConditionTrue(pod.Status) && pod.Status.PodIP != "" {
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

func (r *PodReconciler) CloseChannels(logger logr.Logger,
	ctx context.Context,
	pod corev1.Pod,
	ldsChan chan state.SignalMessageOnLdsChannels,
	cdsChan chan state.SignalMessageOnCdsChannels) {
	for {
		p := corev1.Pod{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}, &p); err != nil {
			if apierrors.IsNotFound(err) {
				logger.Info("Pod fully removed from cluster, closing its ServiceDiscovery channels and from stored Pods")
				//TODO this indexing tactic won't work in a case where multiple pods are being deleted
				state.ClusterState.Pods = remove(state.ClusterState.Pods, pod.UID)
				if ldsChan != nil {
					logger.Info("closing lds channel")
					close(ldsChan)
				}
				if cdsChan != nil {
					logger.Info("closing cds channel")
					close(cdsChan)
				}
				for _, e := range state.ConnectedEdsClients {
					if e.UID == string(pod.UID) {
						close(e.Channel)
						logger.Info("Closing eds channel")
					}
				}
				state.PodChannel <- state.SignalMessageOnPodChannel{
					Verb: 1,
					Pod:  &pod,
				}
				logger.Info("Successfully removed pod from Pods", "pod", pod.Name, "uid", pod.UID, "num of pods left", len(state.ClusterState.Pods))
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

func remove(pods []corev1.Pod, uid types.UID) []corev1.Pod {
	for i, p := range pods {
		if p.UID == uid {
			pods[i] = pods[len(pods)-1]
			return pods[:len(pods)-1]
		}
	}
	ctrl.Log.WithName("POD_DEL").Info("Pod might have been removed previously, this should not happen")
	return pods
}

/*
Had to import the functions below from the k8s.io/kubectl/ package.
It is not working for months now because there's a bump between package versions
*/
// IsPodReadyConditionTrue returns true if a pod is ready; false otherwise.
func isPodReadyConditionTrue(status corev1.PodStatus) bool {
	condition := getPodReadyCondition(status)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

// GetPodReadyCondition extracts the pod ready condition from the given status and returns that.
// Returns nil if the condition is not present.
func getPodReadyCondition(status corev1.PodStatus) *corev1.PodCondition {
	_, condition := getPodCondition(&status, corev1.PodReady)
	return condition
}

// GetPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func getPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return getPodConditionFromList(status.Conditions, conditionType)
}

// GetPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func getPodConditionFromList(conditions []corev1.PodCondition, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}
