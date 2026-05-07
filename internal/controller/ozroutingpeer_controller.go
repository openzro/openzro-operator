package controller

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/go-logr/logr"
	openzrov1 "github.com/openzro/openzro-operator/api/v1"
	"github.com/openzro/openzro-operator/internal/util"
	openzro "github.com/openzro/openzro/management/client/rest"
	"github.com/openzro/openzro/management/server/http/api"
)

// OZRoutingPeerReconciler reconciles a OZRoutingPeer object
type OZRoutingPeerReconciler struct {
	client.Client

	OpenZro            *openzro.Client
	ClientImage        string
	ClusterName        string
	ManagementURL      string
	NamespacedNetworks bool
	DefaultLabels      map[string]string
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *OZRoutingPeerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	logger := ctrl.Log.WithName("OZRoutingPeer").WithValues("namespace", req.Namespace, "name", req.Name)
	logger.Info("Reconciling OZRoutingPeer")

	ozrp := &openzrov1.OZRoutingPeer{}
	err = r.Get(ctx, req.NamespacedName, ozrp)
	if err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(errKubernetesAPI, "error getting OZRoutingPeer", "err", err)
		}
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	originalNBRP := ozrp.DeepCopy()
	defer func() {
		if originalNBRP.DeletionTimestamp != nil && len(ozrp.Finalizers) == 0 {
			return
		}
		if !originalNBRP.Status.Equal(ozrp.Status) {
			err = r.Client.Status().Update(ctx, ozrp)
			if err != nil {
				logger.Error(errKubernetesAPI, "error updating OZRoutingPeer Status", "err", err)
			}
		}
		if err != nil {
			// double check result is nil, otherwise error is not printed
			// and exponential backoff doesn't work properly
			res = ctrl.Result{}
			return
		}
		if !res.Requeue && res.RequeueAfter == 0 {
			res.RequeueAfter = defaultRequeueAfter
		}
	}()

	if ozrp.DeletionTimestamp != nil {
		if len(ozrp.Finalizers) == 0 {
			return ctrl.Result{}, nil
		}
		return r.handleDelete(ctx, req, ozrp, logger)
	}

	logger.Info("OZRoutingPeer: Checking network")
	err = r.handleNetwork(ctx, req, ozrp, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("OZRoutingPeer: Checking groups")
	nbGroup, result, err := r.handleGroup(ctx, req, ozrp, logger)
	if nbGroup == nil {
		return *result, err
	}

	logger.Info("OZRoutingPeer: Checking setup keys")
	result, err = r.handleSetupKey(ctx, req, ozrp, *nbGroup, logger)
	if result != nil {
		return *result, err
	}

	logger.Info("OZRoutingPeer: Checking network router")
	err = r.handleRouter(ctx, ozrp, *nbGroup, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("OZRoutingPeer: Checking admission exempt")
	err = r.handleAdmissionExempt(ctx, ozrp, *nbGroup, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("OZRoutingPeer: Checking deployment")
	err = r.handleDeployment(ctx, req, ozrp, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	ozrp.Status.Conditions = openzrov1.OZConditionTrue()
	return ctrl.Result{}, nil
}

// handleAdmissionExempt ensures this OZRoutingPeer's auto-group is in
// (or out of) the account's AdmissionExemptGroups list, so admission
// posture-check enforcement doesn't lock the routing peer pods out of
// the mesh. Default exempt=true: routing peers are headless server-side
// workloads that cannot run MDM/EDR agents.
//
// Idempotent: only PUTs the account when the desired set != current
// set, to keep the activity log clean.
func (r *OZRoutingPeerReconciler) handleAdmissionExempt(ctx context.Context, ozrp *openzrov1.OZRoutingPeer, nbGroup openzrov1.OZGroup, logger logr.Logger) error {
	if nbGroup.Status.GroupID == nil {
		return nil
	}
	exempt := true
	if ozrp.Spec.ExemptFromAdmission != nil {
		exempt = *ozrp.Spec.ExemptFromAdmission
	}

	accounts, err := r.OpenZro.Accounts.List(ctx)
	if err != nil {
		logger.Error(erropenZroAPI, "error listing accounts", "err", err)
		return err
	}
	if len(accounts) == 0 {
		return nil
	}
	account := accounts[0]
	current := []string{}
	if account.Settings.AdmissionExemptGroups != nil {
		current = *account.Settings.AdmissionExemptGroups
	}

	groupID := *nbGroup.Status.GroupID
	contains := slices.Contains(current, groupID)

	var desired []string
	switch {
	case exempt && !contains:
		desired = append(append([]string{}, current...), groupID)
	case !exempt && contains:
		// User flipped the flag from true to false. Pull our group
		// out — but only ours; preserve manually-added groups.
		desired = make([]string, 0, len(current))
		for _, g := range current {
			if g != groupID {
				desired = append(desired, g)
			}
		}
	default:
		// Already in desired state.
		return nil
	}

	newSettings := account.Settings
	newSettings.AdmissionExemptGroups = &desired
	_, err = r.OpenZro.Accounts.Update(ctx, account.Id, api.AccountRequest{Settings: newSettings})
	if err != nil {
		logger.Error(erropenZroAPI, "error updating AdmissionExemptGroups", "err", err)
		return err
	}
	logger.Info("AdmissionExemptGroups reconciled", "groupID", groupID, "exempt", exempt)
	return nil
}

// removeAdmissionExempt drops a single group ID from the account's
// AdmissionExemptGroups. Used in handleDelete so the cleanup of an
// OZRoutingPeer also cleans up the exempt-list entry it added.
// Manually-added groups (or groups belonging to other still-existing
// OZRoutingPeers) are preserved.
func (r *OZRoutingPeerReconciler) removeAdmissionExempt(ctx context.Context, groupID string, logger logr.Logger) error {
	accounts, err := r.OpenZro.Accounts.List(ctx)
	if err != nil {
		logger.Error(erropenZroAPI, "error listing accounts", "err", err)
		return err
	}
	if len(accounts) == 0 {
		return nil
	}
	account := accounts[0]
	if account.Settings.AdmissionExemptGroups == nil {
		return nil
	}
	current := *account.Settings.AdmissionExemptGroups
	if !slices.Contains(current, groupID) {
		return nil
	}
	desired := make([]string, 0, len(current))
	for _, g := range current {
		if g != groupID {
			desired = append(desired, g)
		}
	}
	newSettings := account.Settings
	newSettings.AdmissionExemptGroups = &desired
	_, err = r.OpenZro.Accounts.Update(ctx, account.Id, api.AccountRequest{Settings: newSettings})
	if err != nil {
		logger.Error(erropenZroAPI, "error removing from AdmissionExemptGroups", "err", err)
		return err
	}
	logger.Info("AdmissionExemptGroups: removed group on delete", "groupID", groupID)
	return nil
}

// handleDeployment reconcile routing peer Deployment
func (r *OZRoutingPeerReconciler) handleDeployment(ctx context.Context, req ctrl.Request, ozrp *openzrov1.OZRoutingPeer, logger logr.Logger) error {
	routingPeerDeployment := appsv1.Deployment{}
	err := r.Client.Get(ctx, req.NamespacedName, &routingPeerDeployment)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(errKubernetesAPI, "error getting Deployment", "err", err)
		ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error getting Deployment: %v", err))
		return err
	}

	labels := r.DefaultLabels
	maps.Copy(labels, ozrp.Spec.Labels)
	podLabels := labels
	podLabels["app.kubernetes.io/name"] = "openzro-router"

	// Create deployment
	if errors.IsNotFound(err) {
		var replicas int32 = 3
		if ozrp.Spec.Replicas != nil {
			replicas = *ozrp.Spec.Replicas
		}
		routingPeerDeployment = appsv1.Deployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      ozrp.Name,
				Namespace: ozrp.Namespace,
				OwnerReferences: []v1.OwnerReference{
					{
						APIVersion:         openzrov1.GroupVersion.Identifier(),
						Kind:               "OZRoutingPeer",
						Name:               ozrp.Name,
						UID:                ozrp.UID,
						BlockOwnerDeletion: util.Ptr(true),
					},
				},
				Labels:      labels,
				Annotations: ozrp.Spec.Annotations,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"app.kubernetes.io/name": "openzro-router",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: v1.ObjectMeta{
						Labels: podLabels,
					},
					Spec: corev1.PodSpec{
						NodeSelector: ozrp.Spec.NodeSelector,
						Tolerations:  ozrp.Spec.Tolerations,
						Containers: []corev1.Container{
							{
								Name:  "openzro",
								Image: r.ClientImage,
								Env: []corev1.EnvVar{
									{
										Name: "OZ_SETUP_KEY",
										ValueFrom: &corev1.EnvVarSource{
											SecretKeyRef: &corev1.SecretKeySelector{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: ozrp.Name,
												},
												Key: "setupKey",
											},
										},
									},
									{
										Name:  "OZ_MANAGEMENT_URL",
										Value: r.ManagementURL,
									},
								},
								SecurityContext: r.buildSecurityContext(ozrp),
								Resources:       ozrp.Spec.Resources,
								VolumeMounts:    ozrp.Spec.VolumeMounts,
							},
						},
						Volumes: ozrp.Spec.Volumes,
					},
				},
			},
		}

		err = r.Client.Create(ctx, &routingPeerDeployment)
		if err != nil {
			logger.Error(errKubernetesAPI, "error creating Deployment", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error creating Deployment: %v", err))
			return err
		}
	} else if err == nil {
		updatedDeployment := routingPeerDeployment.DeepCopy()
		updatedDeployment.ObjectMeta.Name = ozrp.Name
		updatedDeployment.ObjectMeta.Namespace = ozrp.Namespace
		updatedDeployment.ObjectMeta.OwnerReferences = []v1.OwnerReference{
			{
				APIVersion:         openzrov1.GroupVersion.Identifier(),
				Kind:               "OZRoutingPeer",
				Name:               ozrp.Name,
				UID:                ozrp.UID,
				BlockOwnerDeletion: util.Ptr(true),
			},
		}
		updatedDeployment.ObjectMeta.Labels = labels
		for k, v := range ozrp.Spec.Annotations {
			updatedDeployment.ObjectMeta.Annotations[k] = ozrp.Spec.Annotations[v]
		}
		var replicas int32 = 3
		if ozrp.Spec.Replicas != nil {
			replicas = *ozrp.Spec.Replicas
		}
		updatedDeployment.Spec.Replicas = &replicas
		updatedDeployment.Spec.Selector = &v1.LabelSelector{
			MatchLabels: map[string]string{
				"app.kubernetes.io/name": "openzro-router",
			},
		}
		updatedDeployment.Spec.Template.Spec.Tolerations = ozrp.Spec.Tolerations
		updatedDeployment.Spec.Template.Spec.NodeSelector = ozrp.Spec.NodeSelector
		updatedDeployment.Spec.Template.ObjectMeta.Labels = podLabels
		updatedDeployment.Spec.Template.Spec.Volumes = ozrp.Spec.Volumes
		if len(updatedDeployment.Spec.Template.Spec.Containers) != 1 {
			updatedDeployment.Spec.Template.Spec.Containers = []corev1.Container{{}}
		}
		updatedDeployment.Spec.Template.Spec.Containers[0].Name = "openzro"
		updatedDeployment.Spec.Template.Spec.Containers[0].Image = r.ClientImage
		updatedDeployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
			{
				Name: "OZ_SETUP_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: ozrp.Name,
						},
						Key: "setupKey",
					},
				},
			},
			{
				Name:  "OZ_MANAGEMENT_URL",
				Value: r.ManagementURL,
			},
		}
		updatedDeployment.Spec.Template.Spec.Containers[0].SecurityContext = r.buildSecurityContext(ozrp)
		updatedDeployment.Spec.Template.Spec.Containers[0].Resources = ozrp.Spec.Resources
		updatedDeployment.Spec.Template.Spec.Containers[0].VolumeMounts = ozrp.Spec.VolumeMounts

		patch := client.StrategicMergeFrom(&routingPeerDeployment)
		bs, _ := patch.Data(updatedDeployment)
		// To ensure no useless patching is done to the deployment being watched
		// Minimum patch size is 2 for "{}"
		if len(bs) <= 2 {
			return nil
		}
		err = r.Client.Patch(ctx, updatedDeployment, patch)
		if err != nil {
			logger.Error(errKubernetesAPI, "error updating Deployment", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error updating Deployment: %v", err))
			return err
		}
	}

	return err
}

// handleRouter reconcile network routing peer in openZro management API
func (r *OZRoutingPeerReconciler) handleRouter(ctx context.Context, ozrp *openzrov1.OZRoutingPeer, nbGroup openzrov1.OZGroup, logger logr.Logger) error {
	// Check NetworkRouter exists
	routers, err := r.OpenZro.Networks.Routers(*ozrp.Status.NetworkID).List(ctx)

	if err != nil {
		logger.Error(erropenZroAPI, "error listing network routers", "err", err)
		ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error listing network routers: %v", err))
		return err
	}

	if ozrp.Status.RouterID == nil || len(routers) == 0 {
		if len(routers) > 0 {
			// Router exists but isn't saved to status
			ozrp.Status.RouterID = &routers[0].Id
		} else {
			// Create network router
			router, err := r.OpenZro.Networks.Routers(*ozrp.Status.NetworkID).Create(ctx, api.NetworkRouterRequest{
				Enabled:    true,
				Masquerade: true,
				Metric:     9999,
				PeerGroups: &[]string{*nbGroup.Status.GroupID},
			})

			if err != nil {
				logger.Error(erropenZroAPI, "error creating network router", "err", err)
				ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error creating network router: %v", err))
				return err
			}

			ozrp.Status.RouterID = &router.Id
		}
	} else {
		// Ensure network router settings are correct
		if !routers[0].Enabled || !routers[0].Masquerade || routers[0].Metric != 9999 || len(*routers[0].PeerGroups) != 1 || (*routers[0].PeerGroups)[0] != *nbGroup.Status.GroupID {
			_, err = r.OpenZro.Networks.Routers(*ozrp.Status.NetworkID).Update(ctx, routers[0].Id, api.NetworkRouterRequest{
				Enabled:    true,
				Masquerade: true,
				Metric:     9999,
				PeerGroups: &[]string{*nbGroup.Status.GroupID},
			})

			if err != nil {
				logger.Error(erropenZroAPI, "error updating network router", "err", err)
				ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error updating network router: %v", err))
				return err
			}
		}
	}

	return nil
}

// handleSetupKey reconcile setup key and regenerate if invalid
func (r *OZRoutingPeerReconciler) handleSetupKey(ctx context.Context, req ctrl.Request, ozrp *openzrov1.OZRoutingPeer, nbGroup openzrov1.OZGroup, logger logr.Logger) (*ctrl.Result, error) {
	networkName := r.ClusterName
	if r.NamespacedNetworks {
		networkName += "-" + req.Namespace
	}

	// Check if setup key exists
	if ozrp.Status.SetupKeyID == nil {
		// Create new setup key with group Status.GroupID. Default
		// Ephemeral=false so transient gRPC blips don't cause the
		// management to delete the peer 10 minutes after disconnect
		// — that bug looks like "routing peers disappear" from the
		// dashboard while the K8s pods are still running fine.
		ephemeral := false
		if ozrp.Spec.Ephemeral != nil {
			ephemeral = *ozrp.Spec.Ephemeral
		}
		setupKey, err := r.OpenZro.SetupKeys.Create(ctx, api.CreateSetupKeyRequest{
			AutoGroups: []string{*nbGroup.Status.GroupID},
			Ephemeral:  util.Ptr(ephemeral),
			Name:       networkName,
			Type:       "reusable",
		})

		if err != nil {
			logger.Error(erropenZroAPI, "error creating setup key", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error creating setup key: %v", err))
			return &ctrl.Result{}, err
		}

		ozrp.Status.SetupKeyID = &setupKey.Id

		skSecret := corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      ozrp.Name,
				Namespace: ozrp.Namespace,
				OwnerReferences: []v1.OwnerReference{
					{
						APIVersion:         openzrov1.GroupVersion.Identifier(),
						Kind:               "OZRoutingPeer",
						Name:               ozrp.Name,
						UID:                ozrp.UID,
						BlockOwnerDeletion: util.Ptr(true),
					},
				},
				Labels: r.DefaultLabels,
			},
			StringData: map[string]string{
				"setupKey": setupKey.Key,
			},
		}
		err = r.Client.Create(ctx, &skSecret)
		if errors.IsAlreadyExists(err) {
			err = r.Client.Get(ctx, req.NamespacedName, &skSecret)
			if err != nil {
				logger.Error(erropenZroAPI, "error getting secret", "err", err)
				return &ctrl.Result{}, err
			}
			skSecret.Data = map[string][]byte{
				"setupKey": []byte(setupKey.Key),
			}
			err = r.Client.Update(ctx, &skSecret)
		}

		if err != nil {
			logger.Error(errKubernetesAPI, "error creating Secret", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error creating secret: %v", err))
			return &ctrl.Result{}, err
		}
	} else {
		// Check SetupKey is not revoked
		setupKey, err := r.OpenZro.SetupKeys.Get(ctx, *ozrp.Status.SetupKeyID)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			logger.Error(erropenZroAPI, "error getting setup key", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error getting setup key: %v", err))
			return &ctrl.Result{}, err
		}

		if (err != nil && strings.Contains(err.Error(), "not found")) || setupKey == nil || setupKey.Revoked {
			if setupKey != nil && setupKey.Revoked {
				err = r.OpenZro.SetupKeys.Delete(ctx, *ozrp.Status.SetupKeyID)

				if err != nil {
					logger.Error(erropenZroAPI, "error deleting setup key", "err", err)
					ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error deleting setup key: %v", err))
					return &ctrl.Result{}, err
				}
			}

			ozrp.Status.SetupKeyID = nil
			// Requeue to avoid repeating code
			return &ctrl.Result{Requeue: true}, nil
		}

		// Check if secret is valid
		skSecret := corev1.Secret{}
		err = r.Client.Get(ctx, req.NamespacedName, &skSecret)
		if err != nil && !errors.IsNotFound(err) {
			logger.Error(errKubernetesAPI, "error getting Secret", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error getting secret: %v", err))
			return &ctrl.Result{}, err
		}

		if _, ok := skSecret.Data["setupKey"]; errors.IsNotFound(err) || !ok {
			// Someone deleted setup key secret
			// Revoke SK from openZro and re-generate
			err = r.OpenZro.SetupKeys.Delete(ctx, *ozrp.Status.SetupKeyID)

			if err != nil {
				logger.Error(erropenZroAPI, "error deleting setup key", "err", err)
				ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error deleting setup key: %v", err))
				return &ctrl.Result{}, err
			}

			ozrp.Status.SetupKeyID = nil

			ozrp.Status.Conditions = openzrov1.OZConditionFalse("Gone", "generated secret was deleted")
			// Requeue to avoid repeating code
			return &ctrl.Result{Requeue: true}, nil
		}
	}

	return nil, nil
}

// handleGroup creates/updates OZGroup for routing peer
func (r *OZRoutingPeerReconciler) handleGroup(ctx context.Context, req ctrl.Request, ozrp *openzrov1.OZRoutingPeer, logger logr.Logger) (*openzrov1.OZGroup, *ctrl.Result, error) {
	networkName := r.ClusterName
	if r.NamespacedNetworks {
		networkName += "-" + req.Namespace
	}

	// Check if openZro Group exists
	nbGroup := openzrov1.OZGroup{}
	err := r.Client.Get(ctx, req.NamespacedName, &nbGroup)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(errKubernetesAPI, "error getting OZGroup", "err", err)
		ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error getting OZGroup: %v", err))
		return nil, &ctrl.Result{}, err
	}

	if errors.IsNotFound(err) {
		nbGroup = openzrov1.OZGroup{
			ObjectMeta: v1.ObjectMeta{
				Name:      ozrp.Name,
				Namespace: ozrp.Namespace,
				OwnerReferences: []v1.OwnerReference{
					{
						APIVersion:         openzrov1.GroupVersion.Identifier(),
						Kind:               "OZRoutingPeer",
						Name:               ozrp.Name,
						UID:                ozrp.UID,
						BlockOwnerDeletion: util.Ptr(true),
					},
				},
				Finalizers: []string{"openzro.io/group-cleanup", "openzro.io/routing-peer-cleanup"},
				Labels:     r.DefaultLabels,
			},
			Spec: openzrov1.OZGroupSpec{
				Name: networkName,
			},
		}

		err = r.Client.Create(ctx, &nbGroup)

		if err != nil {
			logger.Error(errKubernetesAPI, "error creating OZGroup", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("internalError", fmt.Sprintf("error creating OZGroup: %v", err))
			return nil, &ctrl.Result{}, err
		}

		// Requeue after 5 seconds to ensure group creation is successful by OZGroup controller.
		return nil, &ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if nbGroup.Status.GroupID == nil {
		// Group is not yet created successfully, requeue
		return nil, &ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return &nbGroup, nil, nil
}

// handleNetwork Create/Update openZro Network
func (r *OZRoutingPeerReconciler) handleNetwork(ctx context.Context, req ctrl.Request, ozrp *openzrov1.OZRoutingPeer, logger logr.Logger) error {
	networkName := r.ClusterName
	if r.NamespacedNetworks {
		networkName += "-" + req.Namespace
	}

	if ozrp.Status.NetworkID == nil {
		// Check if network exists
		networks, err := r.OpenZro.Networks.List(ctx)
		if err != nil {
			logger.Error(erropenZroAPI, "error listing networks", "err", err)
			ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error listing networks: %v", err))
			return err
		}
		var network *api.Network
		for _, n := range networks {
			if n.Name == networkName {
				logger.Info("network already exists", "network-id", n.Id)
				network = &n
			}
		}

		if network != nil {
			ozrp.Status.NetworkID = &network.Id
		} else {
			logger.Info("creating network", "name", networkName)
			network, err := r.OpenZro.Networks.Create(ctx, api.NetworkRequest{
				Name:        networkName,
				Description: &networkDescription,
			})
			if err != nil {
				logger.Error(erropenZroAPI, "error creating network", "err", err)
				ozrp.Status.Conditions = openzrov1.OZConditionFalse("APIError", fmt.Sprintf("error creating network: %v", err))
				return err
			}

			ozrp.Status.NetworkID = &network.Id
		}
	}
	return nil
}

func (r *OZRoutingPeerReconciler) handleDelete(ctx context.Context, req ctrl.Request, ozrp *openzrov1.OZRoutingPeer, logger logr.Logger) (ctrl.Result, error) {
	nbDeployment := appsv1.Deployment{}
	err := r.Client.Get(ctx, req.NamespacedName, &nbDeployment)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(errKubernetesAPI, "error getting Deployment", "err", err)
		return ctrl.Result{}, err
	}
	if err == nil {
		err = r.Client.Delete(ctx, &nbDeployment)
		if err != nil {
			logger.Error(errKubernetesAPI, "error deleting Deployment", "err", err)
			return ctrl.Result{}, err
		}
	}

	if ozrp.Status.SetupKeyID != nil {
		logger.Info("Deleting setup key", "id", *ozrp.Status.SetupKeyID)
		err = r.OpenZro.SetupKeys.Delete(ctx, *ozrp.Status.SetupKeyID)
		if err != nil && !strings.Contains(err.Error(), "not found") {
			logger.Error(erropenZroAPI, "error deleting setupKey", "err", err)
			return ctrl.Result{}, err
		}

		setupKeyID := *ozrp.Status.SetupKeyID
		ozrp.Status.SetupKeyID = nil
		logger.Info("Setup key deleted", "id", setupKeyID)
	}

	nbGroup := openzrov1.OZGroup{}
	err = r.Client.Get(ctx, req.NamespacedName, &nbGroup)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(errKubernetesAPI, "error getting OZGroup", "err", err)
		return ctrl.Result{}, err
	}

	if ozrp.Status.NetworkID != nil {
		nbResourceList := openzrov1.OZResourceList{}
		err = r.Client.List(ctx, &nbResourceList)
		if err != nil {
			logger.Error(errKubernetesAPI, "error listing OZResource", "err", err)
			return ctrl.Result{}, err
		}

		for _, ozrs := range nbResourceList.Items {
			if ozrs.Spec.NetworkID == *ozrp.Status.NetworkID {
				logger.Info("Deleting OZResource", "namespace", ozrs.Namespace, "name", ozrs.Name)
				err = r.Client.Delete(ctx, &ozrs)
				if err != nil {
					logger.Error(errKubernetesAPI, "error deleting OZResource", "err", err)
					return ctrl.Result{}, err
				}
			}
		}

		if len(nbResourceList.Items) == 0 {
			logger.Info("Deleting openZro Network", "id", *ozrp.Status.NetworkID)
			err = r.OpenZro.Networks.Delete(ctx, *ozrp.Status.NetworkID)
			if err != nil && !strings.Contains(err.Error(), "not found") {
				logger.Error(erropenZroAPI, "error deleting Network", "err", err)
				return ctrl.Result{}, err
			}

			ozrp.Status.NetworkID = nil
			ozrp.Status.RouterID = nil
		}
	}

	if nbGroup.Spec.Name != "" && slices.Contains(nbGroup.Finalizers, "openzro.io/routing-peer-cleanup") {
		nbGroup.Finalizers = util.Without(nbGroup.Finalizers, "openzro.io/routing-peer-cleanup")
		logger.Info("Removing openzro.io/routing-peer-cleanup finalizer OZGroup", "namespace", nbGroup.Namespace, "name", nbGroup.Name)
		err = r.Client.Update(ctx, &nbGroup)
		if err != nil {
			logger.Error(errKubernetesAPI, "error deleting OZGroup", "err", err)
			return ctrl.Result{}, err
		}
	}

	if nbGroup.Status.GroupID != nil {
		if err := r.removeAdmissionExempt(ctx, *nbGroup.Status.GroupID, logger); err != nil {
			return ctrl.Result{}, err
		}
	}

	if ozrp.Status.NetworkID != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if len(ozrp.Finalizers) > 0 {
		logger.Info("Removing finalizers", "namespace", ozrp.Namespace, "name", ozrp.Name)
		ozrp.Finalizers = nil
		err = r.Client.Update(ctx, ozrp)
		if err != nil {
			logger.Error(errKubernetesAPI, "error updating OZRoutingPeer finalizers", "err", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// buildSecurityContext creates the appropriate SecurityContext based on the OZRoutingPeer spec
func (r *OZRoutingPeerReconciler) buildSecurityContext(ozrp *openzrov1.OZRoutingPeer) *corev1.SecurityContext {
	securityContext := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{
				"NET_ADMIN",
			},
		},
	}

	// Set privileged mode if specified
	if ozrp.Spec.Privileged != nil && *ozrp.Spec.Privileged {
		securityContext.Privileged = ozrp.Spec.Privileged
	}

	return securityContext
}

// SetupWithManager sets up the controller with the Manager.
func (r *OZRoutingPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openzrov1.OZRoutingPeer{}).
		Named("ozroutingpeer").
		Watches(&appsv1.Deployment{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &openzrov1.OZRoutingPeer{})).
		Watches(&corev1.Secret{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &openzrov1.OZRoutingPeer{})).
		Watches(&openzrov1.OZGroup{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &openzrov1.OZRoutingPeer{})).
		Complete(r)
}
