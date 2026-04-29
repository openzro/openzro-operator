package controller

import (
	"context"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	openzro "github.com/openzro/openzro/management/client/rest"
	"github.com/openzro/openzro/management/server/http/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ozv1alpha1 "github.com/openzro/openzro-operator/api/v1alpha1"
	"github.com/openzro/openzro-operator/internal/k8sutil"
)

type GroupReconciler struct {
	client.Client

	OpenZro *openzro.Client
}

// +kubebuilder:rbac:groups=openzro.io,resources=groups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openzro.io,resources=groups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openzro.io,resources=groups/finalizers,verbs=update
func (r *GroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	group := &ozv1alpha1.Group{}
	err := r.Get(ctx, req.NamespacedName, group)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	sp := patch.NewSerialPatcher(group, r.Client)

	if !group.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, sp, group)
	}

	controllerutil.AddFinalizer(group, k8sutil.Finalizer("group"))
	err = sp.Patch(ctx, group)
	if err != nil {
		return ctrl.Result{}, err
	}

	groupID, err := func() (string, error) {
		groupReq := api.GroupRequest{
			Name: group.Spec.Name,
		}
		if group.Status.GroupID != "" {
			resp, err := r.OpenZro.Groups.Update(ctx, group.Status.GroupID, groupReq)
			if err != nil && !openzro.IsNotFound(err) {
				return "", err
			}
			if err == nil {
				return resp.Id, nil
			}
		}
		resp, err := r.OpenZro.Groups.Create(ctx, groupReq)
		if err != nil {
			return "", err
		}
		return resp.Id, nil
	}()
	if err != nil {
		return ctrl.Result{}, err
	}
	group.Status.GroupID = groupID

	conditions.MarkTrue(group, ozv1alpha1.ReadyCondition, ozv1alpha1.ReconciledReason, "")
	err = sp.Patch(ctx, group, patch.WithStatusObservedGeneration{})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *GroupReconciler) reconcileDelete(ctx context.Context, sp *patch.SerialPatcher, group *ozv1alpha1.Group) (ctrl.Result, error) {
	if group.Status.GroupID != "" {
		err := r.OpenZro.Groups.Delete(ctx, group.Status.GroupID)
		if err != nil && !openzro.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(group, k8sutil.Finalizer("group"))
	err := sp.Patch(ctx, group)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ozv1alpha1.Group{}).
		Complete(r)
}
