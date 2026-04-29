package controller

import (
	"context"
	"errors"
	"time"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	openzro "github.com/openzro/openzro/management/client/rest"
	"github.com/openzro/openzro/management/server/http/api"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	ozv1alpha1 "github.com/openzro/openzro-operator/api/v1alpha1"
	"github.com/openzro/openzro-operator/internal/k8sutil"
	"github.com/openzro/openzro-operator/internal/openzroutil"
)

const (
	SetupKeySecretKey = "setup-key"
)

type SetupKeyReconciler struct {
	client.Client

	OpenZro *openzro.Client
}

// +kubebuilder:rbac:groups=openzro.io,resources=setupkeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openzro.io,resources=setupkeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openzro.io,resources=setupkeys/finalizers,verbs=update
func (r *SetupKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	setupKey := &ozv1alpha1.SetupKey{}
	err := r.Get(ctx, req.NamespacedName, setupKey)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	owner, err := k8sutil.ControllerReference(setupKey, r.Client.Scheme())
	if err != nil {
		return ctrl.Result{}, err
	}
	sp := patch.NewSerialPatcher(setupKey, r.Client)

	if !setupKey.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, sp, setupKey)
	}

	autoGroupIDs, err := openzroutil.GetGroupIDs(ctx, r.Client, r.OpenZro, setupKey.Spec.AutoGroups, setupKey.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.AddFinalizer(setupKey, k8sutil.Finalizer("setupkey"))
	err = sp.Patch(ctx, setupKey)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if setup key is up to date.
	ok, err := func() (bool, error) {
		if setupKey.Status.SetupKeyID == "" {
			return false, nil
		}

		// Check setup key in openZro.
		resp, err := r.OpenZro.SetupKeys.Get(ctx, setupKey.Status.SetupKeyID)
		if openzro.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}

		switch resp.State {
		case "valid":
		case "overused":
			return false, errors.New("setup key is overused")
		default:
			return false, nil
		}

		// Secret exists and has not been modified.
		secret := &corev1.Secret{}
		err = r.Client.Get(ctx, client.ObjectKey{Name: setupKey.SecretName(), Namespace: req.Namespace}, secret)
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if resp.Key[:5] != string(secret.Data[SetupKeySecretKey])[:5] {
			return false, nil
		}

		// Auto groups have not been changed.
		setupKeyReq := api.PutApiSetupKeysKeyIdJSONRequestBody{
			AutoGroups: autoGroupIDs,
		}
		_, err = r.OpenZro.SetupKeys.Update(ctx, setupKey.Status.SetupKeyID, setupKeyReq)
		if err != nil {
			return false, err
		}

		return true, nil
	}()
	if err != nil {
		return ctrl.Result{}, err
	}
	if ok {
		return ctrl.Result{RequeueAfter: 15 * time.Minute}, nil
	}
	oldSetupKeyID := setupKey.Status.SetupKeyID

	// Setup key does not exist so we create one.
	expiresIn := 0
	if setupKey.Spec.Duration != nil {
		expiresIn = int(setupKey.Spec.Duration.Seconds())
	}
	setupKeyReq := api.PostApiSetupKeysJSONRequestBody{
		AllowExtraDnsLabels: ptr.To(false),
		AutoGroups:          autoGroupIDs,
		Ephemeral:           ptr.To(setupKey.Spec.Ephemeral),
		ExpiresIn:           expiresIn,
		Name:                setupKey.Spec.Name,
		Type:                "reusable",
		UsageLimit:          0,
	}
	resp, err := r.OpenZro.SetupKeys.Create(ctx, setupKeyReq)
	if err != nil {
		return ctrl.Result{}, err
	}
	setupKey.Status.SetupKeyID = resp.Id
	err = sp.Patch(ctx, setupKey)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create the secret containing the key.
	data := map[string]string{
		SetupKeySecretKey: resp.Key,
	}
	secret := corev1ac.Secret(setupKey.SecretName(), req.Namespace).
		WithStringData(data).
		WithOwnerReferences(owner)
	err = r.Client.Apply(ctx, secret)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete the old status key if we are recreating.
	if oldSetupKeyID != "" {
		err = r.OpenZro.SetupKeys.Delete(ctx, oldSetupKeyID)
		if err != nil && !openzro.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	conditions.MarkTrue(setupKey, ozv1alpha1.ReadyCondition, ozv1alpha1.ReconciledReason, "")
	err = sp.Patch(ctx, setupKey, patch.WithStatusObservedGeneration{})
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 15 * time.Minute}, nil
}

func (r *SetupKeyReconciler) reconcileDelete(ctx context.Context, sp *patch.SerialPatcher, setupKey *ozv1alpha1.SetupKey) (ctrl.Result, error) {
	if setupKey.Status.SetupKeyID != "" {
		err := r.OpenZro.SetupKeys.Delete(ctx, setupKey.Status.SetupKeyID)
		if err != nil && !openzro.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(setupKey, k8sutil.Finalizer("setupkey"))
	err := sp.Patch(ctx, setupKey)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *SetupKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ozv1alpha1.SetupKey{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
