package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openzrov1 "github.com/openzro/openzro-operator/api/v1"
)

var _ = Describe("NBGroup Webhook", func() {
	var (
		obj       *openzrov1.NBGroup
		oldObj    *openzrov1.NBGroup
		validator NBGroupCustomValidator
	)

	BeforeEach(func() {
		obj = &openzrov1.NBGroup{}
		oldObj = &openzrov1.NBGroup{}
		validator = NBGroupCustomValidator{
			client: k8sClient,
		}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	Context("When creating or updating NBGroup under Validating Webhook", func() {
		It("should allow creation", func() {
			Expect(validator.ValidateCreate(ctx, obj)).Error().NotTo(HaveOccurred())
		})
		It("should allow update", func() {
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().NotTo(HaveOccurred())
		})
		When("There are no owners", func() {
			It("should allow deletion", func() {
				obj = &openzrov1.NBGroup{
					ObjectMeta: v1.ObjectMeta{
						Name:            "test",
						Namespace:       "default",
						OwnerReferences: nil,
					},
				}
				Expect(validator.ValidateDelete(ctx, obj)).Error().NotTo(HaveOccurred())
			})
		})
		When("There deleted owners", func() {
			It("should allow deletion", func() {
				obj = &openzrov1.NBGroup{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
						OwnerReferences: []v1.OwnerReference{
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       "NBResource",
								Name:       "notexist",
								UID:        obj.UID,
							},
						},
					},
				}
				Expect(validator.ValidateDelete(ctx, obj)).Error().NotTo(HaveOccurred())
			})
		})
		When("NBResource owner exists", func() {
			BeforeEach(func() {
				nbResource := &openzrov1.NBResource{
					ObjectMeta: v1.ObjectMeta{
						Name:      "isexist",
						Namespace: "default",
					},
					Spec: openzrov1.NBResourceSpec{
						Name:      "test1",
						NetworkID: "test2",
						Address:   "test3",
						Groups:    []string{"test"},
					},
				}

				Expect(k8sClient.Create(ctx, nbResource)).To(Succeed())

				obj = &openzrov1.NBGroup{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
						OwnerReferences: []v1.OwnerReference{
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       nbResource.Kind,
								Name:       nbResource.Name,
								UID:        nbResource.UID,
							},
						},
					},
				}
			})
			AfterEach(func() {
				nbResource := &openzrov1.NBResource{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "isexist"}, nbResource)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
					if len(nbResource.Finalizers) > 0 {
						nbResource.Finalizers = nil
						Expect(k8sClient.Update(ctx, nbResource)).To(Succeed())
					}
					err = k8sClient.Delete(ctx, nbResource)
					if !errors.IsNotFound(err) {
						Expect(err).NotTo(HaveOccurred())
					}
				}
			})
			It("should deny deletion", func() {
				Expect(validator.ValidateDelete(ctx, obj)).Error().To(HaveOccurred())
			})
		})
		When("NBRoutingPeer owner exists", func() {
			BeforeEach(func() {
				ozrp := &openzrov1.NBRoutingPeer{
					ObjectMeta: v1.ObjectMeta{
						Name:      "isexist",
						Namespace: "default",
					},
					Spec: openzrov1.NBRoutingPeerSpec{},
				}

				Expect(k8sClient.Create(ctx, ozrp)).To(Succeed())

				obj = &openzrov1.NBGroup{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
						OwnerReferences: []v1.OwnerReference{
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       ozrp.Kind,
								Name:       ozrp.Name,
								UID:        ozrp.UID,
							},
						},
					},
				}
			})
			AfterEach(func() {
				ozrp := &openzrov1.NBRoutingPeer{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "isexist"}, ozrp)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())
					if len(ozrp.Finalizers) > 0 {
						ozrp.Finalizers = nil
						Expect(k8sClient.Update(ctx, ozrp)).To(Succeed())
					}
					err = k8sClient.Delete(ctx, ozrp)
					if !errors.IsNotFound(err) {
						Expect(err).NotTo(HaveOccurred())
					}
				}
			})
			It("should deny deletion", func() {
				Expect(validator.ValidateDelete(ctx, obj)).Error().To(HaveOccurred())
			})
		})
	})

})
