package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openzrov1 "github.com/openzro/openzro-operator/api/v1"
	"github.com/openzro/openzro-operator/internal/util"
	openzro "github.com/openzro/openzro/management/client/rest"
	"github.com/openzro/openzro/management/server/http/api"
)

var _ = Describe("OZGroup Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		var openzroClient *openzro.Client
		var mux *http.ServeMux
		var server *httptest.Server
		var nbGroup openzrov1.OZGroup

		BeforeEach(func() {
			mux = &http.ServeMux{}
			server = httptest.NewServer(mux)
			openzroClient = openzro.New(server.URL, "ABC")

			err := k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
			if err == nil {
				deleteErr := k8sClient.Delete(ctx, &nbGroup)
				Expect(deleteErr).NotTo(HaveOccurred())
			}
			if err == nil || errors.IsNotFound(err) {
				nbGroup = openzrov1.OZGroup{
					ObjectMeta: v1.ObjectMeta{
						Name:       resourceName,
						Namespace:  typeNamespacedName.Namespace,
						Finalizers: []string{"openzro.io/group-cleanup"},
					},
					Spec: openzrov1.OZGroupSpec{
						Name: resourceName,
					},
				}
				err = k8sClient.Create(ctx, &nbGroup)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			server.Close()
			resource := &openzrov1.OZGroup{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			if len(resource.Finalizers) > 0 {
				resource.Finalizers = nil
				Expect(k8sClient.Update(ctx, resource)).To(Succeed())
			}

			if resource.DeletionTimestamp == nil {
				By("Cleanup the specific resource instance OZGroup")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		When("Group doesn't exist", func() {
			It("should create group", func() {
				By("Reconciling the created resource")
				controllerReconciler := &OZGroupReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						_, err := w.Write([]byte("[]"))
						Expect(err).NotTo(HaveOccurred())
					} else {
						resp := api.Group{
							Id:   "Test",
							Name: resourceName,
						}
						bs, err := json.Marshal(resp)
						Expect(err).NotTo(HaveOccurred())
						_, err = w.Write(bs)
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
				Expect(err).NotTo(HaveOccurred())
				Expect(nbGroup.Status.GroupID).NotTo(BeNil())
				Expect(*nbGroup.Status.GroupID).To(Equal("Test"))
				Expect(nbGroup.Status.Conditions).To(HaveLen(1))
				Expect(nbGroup.Status.Conditions[0].Status).To(BeEquivalentTo(v1.ConditionTrue))
				Expect(nbGroup.Status.Conditions[0].Type).To(Equal(openzrov1.OZSetupKeyReady))
			})
		})

		When("Group already exists", func() {
			It("should use existing group", func() {
				By("Reconciling the created resource")
				controllerReconciler := &OZGroupReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "Test",
							Name: resourceName,
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
				Expect(err).NotTo(HaveOccurred())
				Expect(nbGroup.Status.GroupID).NotTo(BeNil())
				Expect(*nbGroup.Status.GroupID).To(Equal("Test"))
				Expect(nbGroup.Status.Conditions).To(HaveLen(1))
				Expect(nbGroup.Status.Conditions[0].Status).To(BeEquivalentTo(v1.ConditionTrue))
				Expect(nbGroup.Status.Conditions[0].Type).To(Equal(openzrov1.OZSetupKeyReady))
			})
		})

		When("OZGroup is set for deletion", func() {
			deleteGroup := func() {
				GinkgoHelper()
				By("Adding the group ID in status")
				nbGroup.Status.GroupID = util.Ptr("Test")
				err := k8sClient.Status().Update(ctx, &nbGroup)
				Expect(err).NotTo(HaveOccurred())

				By("Deleting the object")
				err = k8sClient.Delete(ctx, &nbGroup)
				Expect(err).NotTo(HaveOccurred())
			}

			When("Group is not linked to any resources", func() {
				It("should delete group", func() {
					deleteGroup()
					By("Reconciling the deleting resource")
					controllerReconciler := &OZGroupReconciler{
						Client:  k8sClient,
						OpenZro: openzroClient,
					}

					method := ""
					mux.HandleFunc("/api/groups/Test", func(w http.ResponseWriter, r *http.Request) {
						method = r.Method
						_, err := w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					})

					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())
					err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
					Expect(errors.IsNotFound(err)).To(BeTrue())
					Expect(method).To(Equal(http.MethodDelete))
				})
			})

			When("Group is linked to other resources", func() {
				It("should return error", func() {
					deleteGroup()
					By("Reconciling the deleting resource")
					controllerReconciler := &OZGroupReconciler{
						Client:  k8sClient,
						OpenZro: openzroClient,
					}

					method := ""
					mux.HandleFunc("/api/groups/Test", func(w http.ResponseWriter, r *http.Request) {
						method = r.Method
						w.WriteHeader(400)
						_, err := w.Write([]byte(`{"message": "group has been linked to Policy: meow"}`))
						Expect(err).NotTo(HaveOccurred())
					})

					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).To(HaveOccurred())
					err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
					Expect(errors.IsNotFound(err)).To(BeFalse())
					Expect(method).To(Equal(http.MethodDelete))
				})
			})

			When("Group already exists in another namespace", func() {
				It("Should delete OZGroup after linked failure", func() {
					deleteGroup()
					otherGroup := &openzrov1.OZGroup{
						ObjectMeta: v1.ObjectMeta{
							Name:      nbGroup.Name,
							Namespace: "kube-system",
						},
						Spec: openzrov1.OZGroupSpec{
							Name: nbGroup.Spec.Name,
						},
					}
					Expect(k8sClient.Create(ctx, otherGroup)).To(Succeed())

					otherGroup.Status.GroupID = nbGroup.Status.GroupID
					Expect(k8sClient.Status().Update(ctx, otherGroup)).To(Succeed())

					By("Reconciling the deleting resource")
					controllerReconciler := &OZGroupReconciler{
						Client:  k8sClient,
						OpenZro: openzroClient,
					}

					method := ""
					mux.HandleFunc("/api/groups/Test", func(w http.ResponseWriter, r *http.Request) {
						method = r.Method
						w.WriteHeader(400)
						_, err := w.Write([]byte(`{"message": "group has been linked to Policy: meow"}`))
						Expect(err).NotTo(HaveOccurred())
					})

					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})

					Expect(err).NotTo(HaveOccurred())
					err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
					Expect(errors.IsNotFound(err)).To(BeTrue())
					Expect(method).To(Equal(http.MethodDelete))
				})
			})
		})

		When("Group already exists with different ID", func() {
			It("should re-use existing group ID", func() {
				controllerReconciler := &OZGroupReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "Test",
							Name: resourceName,
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				nbGroup.Status.GroupID = util.Ptr("Toast")
				Expect(k8sClient.Status().Update(ctx, &nbGroup)).To(Succeed())

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
				Expect(err).NotTo(HaveOccurred())
				Expect(nbGroup.Status.GroupID).NotTo(BeNil())
				Expect(*nbGroup.Status.GroupID).To(Equal("Test"))
				Expect(nbGroup.Status.Conditions).To(HaveLen(1))
				Expect(nbGroup.Status.Conditions[0].Status).To(BeEquivalentTo(v1.ConditionTrue))
				Expect(nbGroup.Status.Conditions[0].Type).To(Equal(openzrov1.OZSetupKeyReady))
			})
		})

		When("Group deleted from openZro API", func() {
			It("Should requeue and create group on next run", func() {
				controllerReconciler := &OZGroupReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						resp := []api.Group{}
						bs, err := json.Marshal(resp)
						Expect(err).NotTo(HaveOccurred())
						_, err = w.Write(bs)
						Expect(err).NotTo(HaveOccurred())
					}
					if r.Method == http.MethodPost {
						resp := api.Group{
							Id:   "Test",
							Name: resourceName,
						}
						bs, err := json.Marshal(resp)
						Expect(err).NotTo(HaveOccurred())
						_, err = w.Write(bs)
						Expect(err).NotTo(HaveOccurred())
					}
				})

				nbGroup.Status.GroupID = util.Ptr("Toast")
				Expect(k8sClient.Status().Update(ctx, &nbGroup)).To(Succeed())

				res, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(res.Requeue).To(BeTrue())

				err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
				Expect(err).NotTo(HaveOccurred())

				Expect(nbGroup.Status.GroupID).To(BeNil())
				Expect(nbGroup.Status.Conditions).To(HaveLen(1))
				Expect(nbGroup.Status.Conditions[0].Status).To(BeEquivalentTo(v1.ConditionFalse))

				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				err = k8sClient.Get(ctx, typeNamespacedName, &nbGroup)
				Expect(err).NotTo(HaveOccurred())

				Expect(nbGroup.Status.GroupID).NotTo(BeNil())
				Expect(*nbGroup.Status.GroupID).To(Equal("Test"))
				Expect(nbGroup.Status.Conditions).To(HaveLen(1))
				Expect(nbGroup.Status.Conditions[0].Status).To(BeEquivalentTo(v1.ConditionTrue))
			})
		})
	})
})
