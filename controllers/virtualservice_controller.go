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
	ds "github.com/davidkornel/operator/controlplane/discoveryservices"
	"github.com/davidkornel/operator/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VirtualServiceReconciler reconciles a VirtualService object
type VirtualServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const vsvcFinalizer = "l7mp.io/delete"

//+kubebuilder:rbac:groups=servicemesh.l7mp.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=servicemesh.l7mp.io,resources=virtualservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=servicemesh.l7mp.io,resources=virtualservices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VirtualService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *VirtualServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("VSVC_CON")

	virtualservice := &l7mpiov1.VirtualService{}
	if err := r.Get(ctx, req.NamespacedName, virtualservice); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since we can get them on deleted requests.
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch VirtualService")
		return ctrl.Result{}, err
	}

	//When a vsvc CR being deleted first it's going to get a Finalizer after that
	//it's going to be deleted. In total the deletion process will make two changes
	//not at the same time, but after each other. Summarized it will rerun the 'reconcile'
	//loop two times. First when it just got a finalizer but not the 'deletionTimestamp'
	//we should ignore the changes.
	if controllerutil.ContainsFinalizer(virtualservice, vsvcFinalizer) {
		if virtualservice.GetDeletionTimestamp() == nil {
			return ctrl.Result{}, nil
		} else {
			//logger.Info("DELETE RESOURCE ", "vsvc", virtualservice.Name)
			if uids, err := state.ClusterState.GetUidListByLabel(logger, virtualservice.Spec.Selector); err == nil {
				for _, uid := range uids {
					state.LdsChannels[uid] <- state.SignalMessageOnLdsChannels{
						Verb:      1, //Delete
						Resources: ds.CreateEnvoyListenerConfigFromVsvcSpec(virtualservice.Spec),
					}
					state.CdsChannels[uid] <- state.SignalMessageOnCdsChannels{
						Verb:      1, //Delete
						Resources: ds.CreateEnvoyClusterConfigFromVsvcSpec(virtualservice.Spec, uid),
					}
				}
			}
			logger.Info("Removing vsvc", "vsvc", virtualservice.Name)
			state.ClusterState.RemoveElementFromSlice(logger, *virtualservice)
			controllerutil.RemoveFinalizer(virtualservice, vsvcFinalizer)
			err := r.Update(ctx, virtualservice)
			if err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}
	//Storing names to know which vsvc are already processed
	//If the rtpe-controller adds the finalizer to the CR, reconcile loop will run again,
	//but this time we don't want to try to create a config out of it, because it's already done
	state.ClusterState.Vsvcs = append(state.ClusterState.Vsvcs, *virtualservice)
	logger.Info("New virtualservice: ", "virtualservice", virtualservice.Name)

	state.VsvcChannel <- virtualservice.Spec

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&l7mpiov1.VirtualService{}).
		Complete(r)
}
