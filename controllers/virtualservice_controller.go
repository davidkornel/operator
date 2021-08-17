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

	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	l7mpiov1 "github.com/davidkornel/operator/api/v1"
)

// VirtualServiceReconciler reconciles a VirtualService object
type VirtualServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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
	logger := log.FromContext(ctx)

	virtualservice := &l7mpiov1.VirtualService{}
	err := r.Get(ctx, req.NamespacedName, virtualservice)
	if err != nil {
		return ctrl.Result{}, err
	} else {
		state.ClusterState.Vsvcs = append(state.ClusterState.Vsvcs, virtualservice.Spec)
		logger.Info("New VSVC: ", "virtualservice", virtualservice.Name)

		state.VsvcChannel <- virtualservice.Spec
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&l7mpiov1.VirtualService{}).
		Complete(r)
}
