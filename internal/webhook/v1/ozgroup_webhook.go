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

// SetupNBGroupWebhookWithManager registers the webhook for OZGroup in the manager.
func SetupNBGroupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openzrov1.OZGroup{}).
		WithValidator(&OZGroupCustomValidator{client: mgr.GetClient()}).
		Complete()
}

// OZGroupCustomValidator struct is responsible for validating the OZGroup resource
// when it is created, updated, or deleted.
type OZGroupCustomValidator struct {
	client client.Client
}

var _ admission.Validator[*openzrov1.OZGroup] = &OZGroupCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type OZGroup.
func (v *OZGroupCustomValidator) ValidateCreate(ctx context.Context, group *openzrov1.OZGroup) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type OZGroup.
func (v *OZGroupCustomValidator) ValidateUpdate(ctx context.Context, old, new *openzrov1.OZGroup) (admission.Warnings, error) {
	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type OZGroup.
func (v *OZGroupCustomValidator) ValidateDelete(ctx context.Context, ozgroup *openzrov1.OZGroup) (admission.Warnings, error) {
	ozgrouplog.Info("Validation for OZGroup upon deletion", "name", ozgroup.GetName())

	for _, o := range ozgroup.OwnerReferences {
		if o.Kind == (&openzrov1.OZResource{}).Kind {
			var nbResource openzrov1.OZResource
			err := v.client.Get(ctx, types.NamespacedName{Namespace: ozgroup.Namespace, Name: o.Name}, &nbResource)
			if err != nil && !errors.IsNotFound(err) {
				return nil, err
			}
			if err == nil && nbResource.DeletionTimestamp == nil {
				return nil, fmt.Errorf("group attached to OZResource %s/%s", ozgroup.Namespace, o.Name)
			}
		}
		if o.Kind == (&openzrov1.OZRoutingPeer{}).Kind {
			var nbResource openzrov1.OZRoutingPeer
			err := v.client.Get(ctx, types.NamespacedName{Namespace: ozgroup.Namespace, Name: o.Name}, &nbResource)
			if err != nil && !errors.IsNotFound(err) {
				return nil, err
			}
			if err == nil && nbResource.DeletionTimestamp == nil {
				return nil, fmt.Errorf("group attached to OZRoutingPeer %s/%s", ozgroup.Namespace, o.Name)
			}
		}
	}

	return nil, nil
}
