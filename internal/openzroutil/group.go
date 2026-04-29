package openzroutil

import (
	"context"
	"fmt"

	openzro "github.com/openzro/openzro/management/client/rest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ozv1alpha1 "github.com/openzro/openzro-operator/api/v1alpha1"
)

func GetGroupIDs(ctx context.Context, k8sClient client.Client, nbClient *openzro.Client, refs []ozv1alpha1.GroupReference, namespace string) ([]string, error) {
	groupIDs := []string{}
	for _, ref := range refs {
		switch {
		case ref.Name != nil:
			group, err := nbClient.Groups.GetByName(ctx, *ref.Name)
			if err != nil {
				return nil, err
			}
			groupIDs = append(groupIDs, group.Id)
		case ref.ID != nil:
			group, err := nbClient.Groups.Get(ctx, *ref.ID)
			if err != nil {
				return nil, err
			}
			groupIDs = append(groupIDs, group.Id)
		case ref.LocalRef != nil:
			group := ozv1alpha1.Group{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ref.LocalRef.Name,
					Namespace: namespace,
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&group), &group)
			if err != nil {
				return nil, err
			}
			if group.Status.GroupID == "" {
				return nil, fmt.Errorf("group %s in groups list is not ready", group.Name)
			}
			groupIDs = append(groupIDs, group.Status.GroupID)
		}
	}
	return groupIDs, nil
}
