package v1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openzrov1 "github.com/openzro/openzro-operator/api/v1"
)

// nolint:unused
// log is for logging in this package.
var ozgrouplog = logf.Log.WithName("ozgroup-resource")

// SetupNBGroupWebhookWithManager registers the webhook for NBGroup in the manager.
func SetupNBGroupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openzrov1.NBGroup{}).
		WithValidator(&NBGroupCustomValidator{client: mgr.GetClient()}).
		Complete()
}

// NBGroupCustomValidator struct is responsible for validating the NBGroup resource
// when it is created, updated, or deleted.
type NBGroupCustomValidator struct {
	client client.Client
}

var _ admission.Validator[*openzrov1.NBGroup] = &NBGroupCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type NBGroup.
func (v *NBGroupCustomValidator) ValidateCreate(ctx context.Context, group *openzrov1.NBGroup) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type NBGroup.
func (v *NBGroupCustomValidator) ValidateUpdate(ctx context.Context, old, new *openzrov1.NBGroup) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type NBGroup.
func (v *NBGroupCustomValidator) ValidateDelete(ctx context.Context, ozgroup *openzrov1.NBGroup) (admission.Warnings, error) {
	ozgrouplog.Info("Validation for NBGroup upon deletion", "name", ozgroup.GetName())

	for _, o := range ozgroup.OwnerReferences {
		if o.Kind == (&openzrov1.NBResource{}).Kind {
			var nbResource openzrov1.NBResource
			err := v.client.Get(ctx, types.NamespacedName{Namespace: ozgroup.Namespace, Name: o.Name}, &nbResource)
			if err != nil && !errors.IsNotFound(err) {
				return nil, err
			}
			if err == nil && nbResource.DeletionTimestamp == nil {
				return nil, fmt.Errorf("group attached to NBResource %s/%s", ozgroup.Namespace, o.Name)
			}
		}
		if o.Kind == (&openzrov1.NBRoutingPeer{}).Kind {
			var nbResource openzrov1.NBRoutingPeer
			err := v.client.Get(ctx, types.NamespacedName{Namespace: ozgroup.Namespace, Name: o.Name}, &nbResource)
			if err != nil && !errors.IsNotFound(err) {
				return nil, err
			}
			if err == nil && nbResource.DeletionTimestamp == nil {
				return nil, fmt.Errorf("group attached to NBRoutingPeer %s/%s", ozgroup.Namespace, o.Name)
			}
		}
	}

	return nil, nil
}
