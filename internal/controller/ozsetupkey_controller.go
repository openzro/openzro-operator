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
	"context"
	"fmt"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openzrov1 "github.com/openzro/openzro-operator/api/v1"
)

// OZSetupKeyReconciler reconciles a OZSetupKey object
type OZSetupKeyReconciler struct {
	client.Client

	ReferencedSecrets map[string]types.NamespacedName
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OZSetupKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("OZSetupKey").WithValues("namespace", req.Namespace, "name", req.Name)
	logger.Info("Reconciling OZSetupKey")

	nbSetupKey := openzrov1.OZSetupKey{}
	err := r.Get(ctx, req.NamespacedName, &nbSetupKey)
	if err != nil {
		logger.Error(fmt.Errorf("internalError"), "error getting OZSetupKey", "err", err)
		return ctrl.Result{}, nil
	}

	if nbSetupKey.Spec.SecretKeyRef.Name == "" || nbSetupKey.Spec.SecretKeyRef.Key == "" {
		logger.Error(fmt.Errorf("invalid OZSetupKey"), "secretKeyRef must contain both secret name and secret key")
		return ctrl.Result{}, r.setStatus(ctx, &nbSetupKey, openzrov1.OZSetupKeyStatus{
			Conditions: []openzrov1.OZCondition{
				{
					Type:          openzrov1.OZSetupKeyReady,
					Status:        corev1.ConditionFalse,
					LastProbeTime: v1.Now(),
					Reason:        "InvalidConfig",
					Message:       "secretKeyRef must contain both secret name and secret key.",
				},
			},
		})
	}

	// Handle updated secret name
	for k, v := range r.ReferencedSecrets {
		if v == req.NamespacedName {
			delete(r.ReferencedSecrets, k)
			break
		}
	}
	r.ReferencedSecrets[fmt.Sprintf("%s/%s", nbSetupKey.Namespace, nbSetupKey.Spec.SecretKeyRef.Name)] = req.NamespacedName

	secret := corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Namespace: nbSetupKey.Namespace, Name: nbSetupKey.Spec.SecretKeyRef.Name}, &secret)
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(fmt.Errorf("internalError"), "error getting secret", "err", err)
			return ctrl.Result{}, err
		}
		logger.Error(fmt.Errorf("invalid OZSetupKey"), "secret referenced not found", "err", err)
		return ctrl.Result{}, r.setStatus(ctx, &nbSetupKey, openzrov1.OZSetupKeyStatus{Conditions: []openzrov1.OZCondition{{
			Type:          openzrov1.OZSetupKeyReady,
			Status:        corev1.ConditionFalse,
			LastProbeTime: v1.Now(),
			Reason:        "SecretNotExists",
			Message:       "Referenced secret does not exist",
		}}})
	}

	uuidBytes, ok := secret.Data[nbSetupKey.Spec.SecretKeyRef.Key]
	if !ok {
		logger.Error(fmt.Errorf("invalid OZSetupKey"), "secret key referenced not found")
		return ctrl.Result{}, r.setStatus(ctx, &nbSetupKey, openzrov1.OZSetupKeyStatus{Conditions: []openzrov1.OZCondition{{
			Type:          openzrov1.OZSetupKeyReady,
			Status:        corev1.ConditionFalse,
			LastProbeTime: v1.Now(),
			Reason:        "SecretKeyNotExists",
			Message:       "Referenced secret key does not exist",
		}}})
	}

	_, err = uuid.Parse(string(uuidBytes))
	if err != nil {
		logger.Error(fmt.Errorf("invalid OZSetupKey"), "setupKey is not a valid UUID", "err", err)
		return ctrl.Result{}, r.setStatus(ctx, &nbSetupKey, openzrov1.OZSetupKeyStatus{Conditions: []openzrov1.OZCondition{{
			Type:          openzrov1.OZSetupKeyReady,
			Status:        corev1.ConditionFalse,
			LastProbeTime: v1.Now(),
			Reason:        "InvalidSetupKey",
			Message:       "Referenced secret is not a valid SetupKey",
		}}})
	}
	return ctrl.Result{}, r.setStatus(ctx, &nbSetupKey, openzrov1.OZSetupKeyStatus{Conditions: []openzrov1.OZCondition{{
		Type:          openzrov1.OZSetupKeyReady,
		Status:        corev1.ConditionTrue,
		LastProbeTime: v1.Now(),
	}}})
}

func (r *OZSetupKeyReconciler) setStatus(ctx context.Context, ozsetupkey *openzrov1.OZSetupKey, status openzrov1.OZSetupKeyStatus) error {
	ozsetupkey.Status = status
	err := r.Status().Update(ctx, ozsetupkey)
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *OZSetupKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.ReferencedSecrets = make(map[string]types.NamespacedName)

	return ctrl.NewControllerManagedBy(mgr).
		For(&openzrov1.OZSetupKey{}).
		Named("ozsetupkey").
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if v, ok := r.ReferencedSecrets[fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())]; ok {
					return []reconcile.Request{
						{
							NamespacedName: v,
						},
					}
				}

				return nil
			}),
		). // Trigger reconciliation when a referenced secret changes
		Complete(r)
}
