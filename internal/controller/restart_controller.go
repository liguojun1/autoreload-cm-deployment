/*
Copyright 2025.

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

package controller

import (
	autov1 "autoreload-cm-deployment/api/v1"
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"
)

// RestartReconciler reconciles a Restart object
type RestartReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=auto.test.com,resources=restarts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=auto.test.com,resources=restarts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=auto.test.com,resources=restarts/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Restart object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *RestartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// TODO(user): your logic here
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, req.NamespacedName, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, fmt.Sprintf("configmap:%s not found", req.Name))
			return ctrl.Result{}, nil
		}
		log.Error(err, fmt.Sprintf("failed to get configmap:%s", req.Name))
		return ctrl.Result{}, err
	}
	if cm.DeletionTimestamp != nil {
		log.Info("configmap is being deleted")
		return ctrl.Result{}, nil
	}
	if cm.Labels == nil {
		log.Info(fmt.Sprintf("configmap:%s not exist labels", cm.Name))
		return ctrl.Result{}, nil
	}
	ref_name, ok := cm.Labels["ref"]
	if !ok {
		log.Info(fmt.Sprintf("configmap:%s not exist ref object", cm.Name))
		return ctrl.Result{}, nil
	}
	ob := &autov1.Restart{}
	err = r.Get(ctx, client.ObjectKey{Name: ref_name, Namespace: "kube-system"}, ob)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("ref:%s not found", ref_name))
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get restart")
		return ctrl.Result{}, err
	}
	exist := false
	for _, i := range ob.Spec.AppList {
		if i == cm.Name {
			exist = true
			break
		}
	}
	if exist == false {
		log.Info(fmt.Sprintf("ref:%s app_list not exist app:%s", ref_name, cm.Name))
		return ctrl.Result{}, nil
	}
	dp := appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: cm.Name, Namespace: cm.Namespace}, &dp)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info(fmt.Sprintf("deployment: %s not found", cm.Name))
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get deployment")
		return ctrl.Result{}, err
	}
	dpCopy := dp.DeepCopy()
	if dpCopy.Spec.Template.Annotations == nil {
		dpCopy.Spec.Template.Annotations = make(map[string]string)
	}
	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	formattedTime := now.Format(time.RFC3339)
	dpCopy.Spec.Template.Annotations["change"] = formattedTime
	r.Recorder.Event(dpCopy, corev1.EventTypeNormal, "开始调谐", "重启dp")
	err = r.Update(ctx, dpCopy)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to update deployment", dpCopy.Name))
		r.Recorder.Event(dpCopy, corev1.EventTypeWarning, "结束调谐", "重启dp失败")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(dpCopy, corev1.EventTypeNormal, "结束调谐", "重启dp完成")
	obCopy := ob.DeepCopy()
	if obCopy.Status.ChangeList == nil {
		obCopy.Status.ChangeList = make(map[string]string)
	}
	obCopy.Status.ChangeList[cm.Name] = formattedTime
	r.Status().Update(ctx, obCopy)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RestartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).WithEventFilter(
		predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				return true
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
		}).
		Complete(r)
}
