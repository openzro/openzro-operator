package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openzrov1 "github.com/openzro/openzro-operator/api/v1"
	"github.com/openzro/openzro-operator/internal/util"
	openzro "github.com/openzro/openzro/shared/management/client/rest"
	"github.com/openzro/openzro/shared/management/http/api"
)

var _ = Describe("NBResource Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"
		const policyGenName = "test-gen"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		ozresource := &openzrov1.NBResource{}
		var openzroClient *openzro.Client
		var mux *http.ServeMux
		var server *httptest.Server
		var controllerReconciler *NBResourceReconciler

		BeforeEach(func() {
			ctrl.SetLogger(logr.New(GinkgoLogr.GetSink()))
			mux = &http.ServeMux{}
			server = httptest.NewServer(mux)
			openzroClient = openzro.New(server.URL, "ABC")
			controllerReconciler = &NBResourceReconciler{
				Client:        k8sClient,
				openZro:       openzroClient,
				ClusterName:   "kubernetes",
				DefaultLabels: map[string]string{"dog": "bark"},
			}

			By("creating the custom resource for the Kind NBResource")
			err := k8sClient.Get(ctx, typeNamespacedName, ozresource)
			if err != nil && errors.IsNotFound(err) {
				ozresource = &openzrov1.NBResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       resourceName,
						Namespace:  "default",
						Finalizers: []string{"openzro.io/cleanup"},
					},
					Spec: openzrov1.NBResourceSpec{
						Name:      "Test",
						NetworkID: "test",
						Address:   "test.default.svc.cluster.local",
						Groups:    []string{"meow"},
						TCPPorts:  []int32{80},
					},
				}
				Expect(k8sClient.Create(ctx, ozresource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &openzrov1.NBResource{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if !errors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())

				if len(resource.Finalizers) > 0 {
					resource.Finalizers = nil
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}

				By("Cleanup the specific resource instance NBResource")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		BeforeEach(func() {
			mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
				defer GinkgoRecover()
				resp := []api.Group{
					{
						Id:   "test",
						Name: "meow",
					},
				}
				bs, err := json.Marshal(resp)
				Expect(err).NotTo(HaveOccurred())
				_, err = w.Write(bs)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("Network Resource doesn't exist", Ordered, func() {
			AfterAll(func() {
				nbGroup := &openzrov1.NBGroup{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, nbGroup)
				if !errors.IsNotFound(err) {
					if len(nbGroup.Finalizers) > 0 {
						nbGroup.Finalizers = nil
						Expect(k8sClient.Update(ctx, nbGroup)).To(Succeed())
					}
					Expect(k8sClient.Delete(ctx, nbGroup)).To(Succeed())
				}
			})

			It("should create NBGroups", func() {
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				nbGroup := &openzrov1.NBGroup{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, nbGroup)).To(Succeed())
				Expect(nbGroup.Labels).To(HaveKeyWithValue("dog", "bark"))
				nbGroup.Status.GroupID = util.Ptr("test")
				Expect(k8sClient.Status().Update(ctx, nbGroup)).To(Succeed())
			})

			It("should create Network Resource", func() {
				networkResourceCreated := false

				mux.HandleFunc("/api/networks/test/resources", func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					if r.Method == http.MethodPost {
						networkResourceCreated = true
						bs, err := io.ReadAll(r.Body)
						Expect(err).NotTo(HaveOccurred())
						var req api.PostApiNetworksNetworkIdResourcesJSONRequestBody
						Expect(json.Unmarshal(bs, &req)).To(Succeed())

						Expect(req.Name).To(Equal("Test"))
						Expect(req.Description).NotTo(BeNil())
						Expect(*req.Description).To(BeEquivalentTo("Created by openzro-operator"))
						Expect(req.Enabled).To(BeTrue())
						Expect(req.Groups).To(ConsistOf([]string{"test"}))
						Expect(req.Address).To(Equal(ozresource.Spec.Address))

						resp := api.NetworkResource{
							Address:     req.Address,
							Description: req.Description,
							Enabled:     req.Enabled,
							Groups: []api.GroupMinimum{
								{
									Id:   "test",
									Name: "meow",
								},
							},
							Id:   "test",
							Name: req.Name,
							Type: api.NetworkResourceTypeDomain,
						}
						bs, err = json.Marshal(resp)
						Expect(err).NotTo(HaveOccurred())
						_, err = w.Write(bs)
						Expect(err).NotTo(HaveOccurred())
					}
				})
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(networkResourceCreated).To(BeTrue())
			})
		})
		When("Network Resource exists", func() {
			BeforeEach(func() {
				ozresource.Status.NetworkResourceID = util.Ptr("test")
				Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

				nbGroup := &openzrov1.NBGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "meow",
						Namespace:  "default",
						Finalizers: []string{"openzro.io/resource-cleanup"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       "NBResource",
								Name:       "test-resource",
								UID:        ozresource.UID,
							},
						},
					},
					Spec: openzrov1.NBGroupSpec{
						Name: "meow",
					},
				}
				Expect(k8sClient.Create(ctx, nbGroup)).To(Succeed())

				nbGroup.Status.GroupID = util.Ptr("test")
				Expect(k8sClient.Status().Update(ctx, nbGroup)).To(Succeed())
			})

			AfterEach(func() {
				nbGroup := &openzrov1.NBGroup{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, nbGroup)
				if !errors.IsNotFound(err) {
					if len(nbGroup.Finalizers) > 0 {
						nbGroup.Finalizers = nil
						Expect(k8sClient.Update(ctx, nbGroup)).To(Succeed())
					}
					Expect(k8sClient.Delete(ctx, nbGroup)).To(Succeed())
				}
			})
			When("Network Resource is out of date", func() {
				It("should update Network Resource", func() {
					resourceUpdated := false
					mux.HandleFunc("/api/networks/test/resources/test", func(w http.ResponseWriter, r *http.Request) {
						defer GinkgoRecover()
						switch r.Method {
						case http.MethodGet:
							resp := api.NetworkResource{
								Address:     ozresource.Spec.Address,
								Description: &networkDescription,
								Enabled:     false,
								Groups: []api.GroupMinimum{
									{
										Id:   "test",
										Name: "meow",
									},
									{
										Id:   "test2",
										Name: "meow2",
									},
								},
								Id:   "test",
								Name: ozresource.Spec.Name,
								Type: api.NetworkResourceTypeDomain,
							}
							bs, err := json.Marshal(resp)
							Expect(err).NotTo(HaveOccurred())
							_, err = w.Write(bs)
							Expect(err).NotTo(HaveOccurred())
						case http.MethodPut:
							resourceUpdated = true
							bs, err := io.ReadAll(r.Body)
							Expect(err).NotTo(HaveOccurred())
							var req api.PutApiNetworksNetworkIdResourcesResourceIdJSONRequestBody
							Expect(json.Unmarshal(bs, &req)).To(Succeed())

							Expect(req.Name).To(Equal("Test"))
							Expect(req.Description).NotTo(BeNil())
							Expect(*req.Description).To(BeEquivalentTo("Created by openzro-operator"))
							Expect(req.Enabled).To(BeTrue())
							Expect(req.Groups).To(ConsistOf([]string{"test"}))
							Expect(req.Address).To(Equal(ozresource.Spec.Address))

							resp := api.NetworkResource{
								Address:     ozresource.Spec.Address,
								Description: &networkDescription,
								Enabled:     true,
								Groups: []api.GroupMinimum{
									{
										Id:   "test",
										Name: "meow",
									},
								},
								Id:   "test",
								Name: ozresource.Spec.Name,
								Type: api.NetworkResourceTypeDomain,
							}
							bs, err = json.Marshal(resp)
							Expect(err).NotTo(HaveOccurred())
							_, err = w.Write(bs)
							Expect(err).NotTo(HaveOccurred())
						}
					})

					ozresource.Status.NetworkResourceID = util.Ptr("test")
					Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

					_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(resourceUpdated).To(BeTrue())
				})
			})

			When("Network Resource is up-to-date", func() {
				BeforeEach(func() {
					mux.HandleFunc("/api/networks/test/resources/test", func(w http.ResponseWriter, r *http.Request) {
						defer GinkgoRecover()
						if r.Method == http.MethodGet {
							resp := api.NetworkResource{
								Address:     ozresource.Spec.Address,
								Description: &networkDescription,
								Enabled:     true,
								Groups: []api.GroupMinimum{
									{
										Id:   "test",
										Name: "meow",
									},
								},
								Id:   "test",
								Name: ozresource.Spec.Name,
								Type: api.NetworkResourceTypeDomain,
							}
							bs, err := json.Marshal(resp)
							Expect(err).NotTo(HaveOccurred())
							_, err = w.Write(bs)
							Expect(err).NotTo(HaveOccurred())
						}
					})
				})

				When("Policy is specified", Ordered, func() {
					When("Policy Exists", func() {
						BeforeAll(func() {
							nbPolicy := &openzrov1.NBPolicy{
								ObjectMeta: metav1.ObjectMeta{
									Name: "test-a",
								},
								Spec: openzrov1.NBPolicySpec{
									Name:         "Test A",
									SourceGroups: []string{"All"},
								},
							}
							Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())

							nbPolicy = &openzrov1.NBPolicy{
								ObjectMeta: metav1.ObjectMeta{
									Name: "test-b",
								},
								Spec: openzrov1.NBPolicySpec{
									Name:         "Test B",
									SourceGroups: []string{"All"},
								},
							}
							Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())
						})

						AfterAll(func() {
							nbPolicy := &openzrov1.NBPolicy{}
							err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)
							if !errors.IsNotFound(err) {
								Expect(k8sClient.Delete(ctx, nbPolicy)).To(Succeed())
							}

							nbPolicy = &openzrov1.NBPolicy{}
							err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-b"}, nbPolicy)
							if !errors.IsNotFound(err) {
								Expect(k8sClient.Delete(ctx, nbPolicy)).To(Succeed())
							}
						})
						It("should update policy status", func() {
							ozresource.Spec.PolicyName = "test-a"
							Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

							_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
								NamespacedName: typeNamespacedName,
							})
							Expect(err).NotTo(HaveOccurred())

							nbPolicy := &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
						})

						When("Policy is updated", func() {
							It("should remove old reference and add new reference", func() {
								ozresource.Spec.PolicyName = "test-b"
								Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

								ozresource.Status.PolicyName = util.Ptr("test-a")
								Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

								_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
									NamespacedName: typeNamespacedName,
								})
								Expect(err).NotTo(HaveOccurred())

								nbPolicy := &openzrov1.NBPolicy{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
								Expect(nbPolicy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))

								nbPolicy = &openzrov1.NBPolicy{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-b"}, nbPolicy)).To(Succeed())
								Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
							})
						})

						When("Policy is removed", func() {
							It("should remove old reference", func() {
								nbPolicy := &openzrov1.NBPolicy{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
								nbPolicy.Status.ManagedServiceList = []string{"default/test-resource"}
								Expect(k8sClient.Status().Update(ctx, nbPolicy)).To(Succeed())

								ozresource.Spec.PolicyName = ""
								Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

								ozresource.Status.PolicyName = util.Ptr("test-a")
								Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

								_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
									NamespacedName: typeNamespacedName,
								})
								Expect(err).NotTo(HaveOccurred())

								nbPolicy = &openzrov1.NBPolicy{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
								Expect(nbPolicy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))
							})
						})
					})

					When("Policy doesn't exist", func() {
						When("Policy auto-creation is enabled", func() {
							BeforeEach(func() {
								controllerReconciler.AllowAutomaticPolicyCreation = true
							})
							AfterEach(func() {
								nbPolicy := &openzrov1.NBPolicy{}
								err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-gen-" + ozresource.Namespace + "-" + ozresource.Name}, nbPolicy)
								if !errors.IsNotFound(err) {
									if len(nbPolicy.Finalizers) > 0 {
										nbPolicy.Finalizers = nil
										Expect(k8sClient.Update(ctx, nbPolicy)).To(Succeed())
									}
									err = k8sClient.Delete(ctx, nbPolicy)
									if !errors.IsNotFound(err) {
										Expect(err).NotTo(HaveOccurred())
									}
								}
							})

							It("should create policy", func() {
								ozresource.Spec.PolicyName = policyGenName
								ozresource.Spec.PolicySourceGroups = []string{"test"}
								Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

								_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
									NamespacedName: typeNamespacedName,
								})
								Expect(err).NotTo(HaveOccurred())

								Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
								Expect(ozresource.Status.PolicyNameMapping).To(HaveKey(policyGenName))

								nbPolicy := &openzrov1.NBPolicy{}
								Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
								Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
								Expect(nbPolicy.Labels).To(HaveKeyWithValue("dog", "bark"))
							})

							When("Source groups is not defined", func() {
								It("should return error", func() {
									ozresource.Spec.PolicyName = policyGenName
									ozresource.Spec.PolicySourceGroups = nil
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).To(HaveOccurred())
								})
							})

							When("Friendly name is specified", func() {
								It("should override policy name", func() {
									ozresource.Spec.PolicyName = policyGenName
									ozresource.Spec.PolicySourceGroups = []string{"test"}
									ozresource.Spec.PolicyFriendlyName = map[string]string{policyGenName: "UnitTest"}
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())

									Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
									Expect(ozresource.Status.PolicyNameMapping).To(HaveKey(policyGenName))

									nbPolicy := &openzrov1.NBPolicy{}
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
									Expect(nbPolicy.Spec.Name).To(Equal("UnitTest"))
								})
							})

							When("Policy already exists", func() {
								It("should update it", func() {
									nbPolicy := &openzrov1.NBPolicy{
										ObjectMeta: metav1.ObjectMeta{
											Name: "test-gen-default-test-resource",
										},
										Spec: openzrov1.NBPolicySpec{
											Name:          "Test",
											Description:   "Test",
											SourceGroups:  []string{"toast"},
											Bidirectional: false,
										},
									}
									Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())

									ozresource.Spec.PolicyName = policyGenName
									ozresource.Spec.PolicySourceGroups = []string{"test"}
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())

									Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
									Expect(ozresource.Status.PolicyNameMapping).To(HaveKey(policyGenName))

									nbPolicy = &openzrov1.NBPolicy{}
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
									Expect(nbPolicy.Spec.Name).To(Equal("Autogenerated policy for resource default/test-resource in cluster kubernetes"))
									Expect(nbPolicy.Spec.Bidirectional).To(BeTrue())
								})
							})

							When("Policy settings are updated", func() {
								It("should update it", func() {
									ozresource.Spec.PolicyName = policyGenName
									ozresource.Spec.PolicySourceGroups = []string{"test"}
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())

									Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
									ozresource.Spec.PolicyFriendlyName = map[string]string{policyGenName: "UnitTest"}

									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())

									Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
									Expect(ozresource.Status.PolicyNameMapping).To(HaveKey(policyGenName))

									nbPolicy := &openzrov1.NBPolicy{}
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
									Expect(nbPolicy.Spec.Name).To(Equal("UnitTest"))
								})
							})

							When("Policy is changed outside controller", func() {
								It("should update it", func() {
									ozresource.Spec.PolicyName = policyGenName
									ozresource.Spec.PolicySourceGroups = []string{"test"}
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())

									Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
									Expect(ozresource.Status.PolicyNameMapping).To(HaveKey(policyGenName))

									nbPolicy := &openzrov1.NBPolicy{}
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									nbPolicy.Spec.Name = "Meow"
									nbPolicy.Annotations = nil
									nbPolicy.Spec.Description = "woeM"
									nbPolicy.Spec.SourceGroups = []string{"est"}
									Expect(k8sClient.Update(ctx, nbPolicy)).To(Succeed())

									_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									Expect(nbPolicy.Spec.Name).To(Equal("Autogenerated policy for resource default/test-resource in cluster kubernetes"))
									Expect(nbPolicy.Annotations["openzro.io/generated-by"]).To(Equal("default/test-resource"))
									Expect(nbPolicy.Spec.Description).To(Equal("Generated by default/test-resource"))
									Expect(nbPolicy.Spec.SourceGroups).To(BeEquivalentTo([]string{"test"}))
								})
							})

							When("Policy is removed", func() {
								It("should delete NBPolicy", func() {
									ozresource.Spec.PolicyName = policyGenName
									ozresource.Spec.PolicySourceGroups = []string{"test"}
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())

									Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
									Expect(ozresource.Status.PolicyNameMapping).To(HaveKey(policyGenName))

									nbPolicy := &openzrov1.NBPolicy{}
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									ozresource.Spec.PolicyName = ""
									Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

									_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
										NamespacedName: typeNamespacedName,
									})
									Expect(err).NotTo(HaveOccurred())
									Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ozresource.Status.PolicyNameMapping[policyGenName]}, nbPolicy)).To(Succeed())
									Expect(nbPolicy.DeletionTimestamp).NotTo(BeNil())
								})
							})
						})
					})
				})

				When("Multiple Policies are specified", Ordered, func() {
					BeforeAll(func() {
						nbPolicy := &openzrov1.NBPolicy{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-a",
							},
							Spec: openzrov1.NBPolicySpec{
								Name:         "Test A",
								SourceGroups: []string{"All"},
							},
						}
						Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())

						nbPolicy = &openzrov1.NBPolicy{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-b",
							},
							Spec: openzrov1.NBPolicySpec{
								Name:         "Test B",
								SourceGroups: []string{"All"},
							},
						}
						Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())

						nbPolicy = &openzrov1.NBPolicy{
							ObjectMeta: metav1.ObjectMeta{
								Name: "test-c",
							},
							Spec: openzrov1.NBPolicySpec{
								Name:         "Test C",
								SourceGroups: []string{"All"},
							},
						}
						Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())
					})

					AfterAll(func() {
						nbPolicy := &openzrov1.NBPolicy{}
						err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)
						if !errors.IsNotFound(err) {
							Expect(k8sClient.Delete(ctx, nbPolicy)).To(Succeed())
						}

						nbPolicy = &openzrov1.NBPolicy{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-b"}, nbPolicy)
						if !errors.IsNotFound(err) {
							Expect(k8sClient.Delete(ctx, nbPolicy)).To(Succeed())
						}

						nbPolicy = &openzrov1.NBPolicy{}
						err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-c"}, nbPolicy)
						if !errors.IsNotFound(err) {
							Expect(k8sClient.Delete(ctx, nbPolicy)).To(Succeed())
						}
					})

					It("should update policies status", func() {
						ozresource.Spec.PolicyName = "test-a, test-b"
						Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

						_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
							NamespacedName: typeNamespacedName,
						})
						Expect(err).NotTo(HaveOccurred())

						nbPolicy := &openzrov1.NBPolicy{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
						Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))

						nbPolicy = &openzrov1.NBPolicy{}
						Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-b"}, nbPolicy)).To(Succeed())
						Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
					})

					When("Policy is updated", func() {
						It("should remove old reference and add new reference", func() {
							ozresource.Spec.PolicyName = "test-b,test-c"
							Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

							ozresource.Status.PolicyName = util.Ptr("test-a,test-b")
							Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

							_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
								NamespacedName: typeNamespacedName,
							})
							Expect(err).NotTo(HaveOccurred())

							nbPolicy := &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))

							nbPolicy = &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-b"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))

							nbPolicy = &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-c"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).To(ContainElement("default/test-resource"))
						})
					})

					When("Policy is removed", func() {
						It("should remove old reference", func() {
							ozresource.Spec.PolicyName = ""
							Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

							ozresource.Status.PolicyName = util.Ptr("test-b,test-c")
							Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

							_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
								NamespacedName: typeNamespacedName,
							})
							Expect(err).NotTo(HaveOccurred())

							nbPolicy := &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-a"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))

							nbPolicy = &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-b"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))

							nbPolicy = &openzrov1.NBPolicy{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-c"}, nbPolicy)).To(Succeed())
							Expect(nbPolicy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))
						})
					})
				})

				When("Groups are changed", func() {
					When("Removed groups are no longer referenced by anything", func() {
						It("should only remove finalizer", func() {
							ozresource.Spec.Groups = []string{"meow2"}
							Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

							_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
								NamespacedName: typeNamespacedName,
							})
							Expect(err).NotTo(HaveOccurred())

							nbGroup := &openzrov1.NBGroup{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, nbGroup)).To(Succeed())
							Expect(nbGroup.Finalizers).To(BeEmpty())
							Expect(nbGroup.OwnerReferences).To(HaveLen(1))
						})
					})

					When("Removed groups are referenced by something else", func() {
						BeforeEach(func() {
							otherResource := &openzrov1.NBResource{
								ObjectMeta: metav1.ObjectMeta{
									Name:      "not-test",
									Namespace: "default",
								},
								Spec: openzrov1.NBResourceSpec{
									Name:      "nottest",
									NetworkID: "test",
									Address:   "test",
									Groups:    []string{"test"},
								},
							}
							Expect(k8sClient.Create(ctx, otherResource)).To(Succeed())

							nbGroup := &openzrov1.NBGroup{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, nbGroup)).To(Succeed())
							nbGroup.OwnerReferences = append(nbGroup.OwnerReferences, metav1.OwnerReference{
								APIVersion: "openzro.io/v1",
								Kind:       "NBResource",
								Name:       "not-test",
								UID:        otherResource.UID,
							})
							Expect(k8sClient.Update(ctx, nbGroup)).To(Succeed())
						})

						AfterEach(func() {
							otherResource := &openzrov1.NBResource{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "not-test"}, otherResource)).To(Succeed())
							Expect(k8sClient.Delete(ctx, otherResource)).To(Succeed())
						})

						It("should only remove owner reference", func() {
							ozresource.Spec.Groups = []string{"meow2"}
							Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

							_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
								NamespacedName: typeNamespacedName,
							})
							Expect(err).NotTo(HaveOccurred())

							nbGroup := &openzrov1.NBGroup{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, nbGroup)).To(Succeed())
							Expect(nbGroup.Finalizers).To(HaveLen(1))
							Expect(nbGroup.OwnerReferences).To(HaveLen(1))
							Expect(nbGroup.OwnerReferences[0].Name).To(Equal("not-test"))
						})
					})
					When("New groups are added", func() {
						It("should create new groups", func() {
							ozresource.Spec.Groups = []string{"meow", "meow3"}
							Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())

							_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
								NamespacedName: typeNamespacedName,
							})
							Expect(err).NotTo(HaveOccurred())

							nbGroup := &openzrov1.NBGroup{}
							Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow3"}, nbGroup)).To(Succeed())
							Expect(nbGroup.OwnerReferences).To(HaveLen(1))
							Expect(nbGroup.Finalizers).To(ConsistOf([]string{"openzro.io/group-cleanup", "openzro.io/resource-cleanup"}))
						})
					})
				})
			})

			When("Network Resource is removed from openZro", func() {
				BeforeEach(func() {
					mux.HandleFunc("/api/networks/test/resources/test", func(w http.ResponseWriter, r *http.Request) {
						defer GinkgoRecover()
						if r.Method == http.MethodGet {
							w.WriteHeader(404)
							_, err := w.Write([]byte(`{"message": "not found", "code": 404}`))
							Expect(err).NotTo(HaveOccurred())
						}
					})
				})

				It("should remove network resource ID and requeue", func() {
					res, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: typeNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(res.Requeue).To(BeTrue())

					Expect(k8sClient.Get(ctx, typeNamespacedName, ozresource)).To(Succeed())
					Expect(ozresource.Status.NetworkResourceID).To(BeNil())
				})
			})
		})
		When("NBResource is set for deletion", Ordered, func() {
			BeforeAll(func() {
				ozresource.Spec.Groups = []string{"meow", "meowdelete"}
				Expect(k8sClient.Update(ctx, ozresource)).To(Succeed())
				ozresource.Status.Groups = []string{"test", "testdelete"}
				ozresource.Status.PolicyName = util.Ptr("test")
				ozresource.Status.NetworkResourceID = util.Ptr("test")
				Expect(k8sClient.Status().Update(ctx, ozresource)).To(Succeed())

				nbPolicy := &openzrov1.NBPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
					Spec: openzrov1.NBPolicySpec{
						Name:          "Test",
						SourceGroups:  []string{"All"},
						Bidirectional: true,
					},
				}
				Expect(k8sClient.Create(ctx, nbPolicy)).To(Succeed())

				nbPolicy.Status.ManagedServiceList = []string{"default/test-resource"}
				Expect(k8sClient.Status().Update(ctx, nbPolicy)).To(Succeed())

				nbGroup := &openzrov1.NBGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "meow",
						Namespace:  "default",
						Finalizers: []string{"openzro.io/resource-cleanup"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       "NBResource",
								Name:       ozresource.Name,
								UID:        ozresource.UID,
							},
						},
					},
					Spec: openzrov1.NBGroupSpec{
						Name: "meow",
					},
				}
				Expect(k8sClient.Create(ctx, nbGroup)).To(Succeed())

				othernbresource := &openzrov1.NBResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-resource",
						Namespace: "default",
					},
					Spec: openzrov1.NBResourceSpec{
						Name:      "test",
						NetworkID: "test",
						Address:   "test",
						Groups:    []string{"test"},
					},
				}
				Expect(k8sClient.Create(ctx, othernbresource)).To(Succeed())

				nbGroup = &openzrov1.NBGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "meowdelete",
						Namespace:  "default",
						Finalizers: []string{"openzro.io/resource-cleanup"},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       "NBResource",
								Name:       ozresource.Name,
								UID:        ozresource.UID,
							},
							{
								APIVersion: openzrov1.GroupVersion.Identifier(),
								Kind:       "NBResource",
								Name:       othernbresource.Name,
								UID:        othernbresource.UID,
							},
						},
					},
					Spec: openzrov1.NBGroupSpec{
						Name: "meow",
					},
				}
				Expect(k8sClient.Create(ctx, nbGroup)).To(Succeed())
			})
			AfterAll(func() {
				policy := &openzrov1.NBPolicy{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test"}, policy)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())

					if len(policy.Finalizers) > 0 {
						policy.Finalizers = nil
						Expect(k8sClient.Update(ctx, policy)).To(Succeed())
					}

					Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
				}

				resource := &openzrov1.NBResource{}
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "other-resource"}, resource)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())

					if len(resource.Finalizers) > 0 {
						resource.Finalizers = nil
						Expect(k8sClient.Update(ctx, resource)).To(Succeed())
					}

					Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				}

				group := &openzrov1.NBGroup{}
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, group)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())

					if len(group.Finalizers) > 0 {
						group.Finalizers = nil
						Expect(k8sClient.Update(ctx, group)).To(Succeed())
					}

					Expect(k8sClient.Delete(ctx, group)).To(Succeed())
				}

				group = &openzrov1.NBGroup{}
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meowdelete"}, group)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())

					if len(group.Finalizers) > 0 {
						group.Finalizers = nil
						Expect(k8sClient.Update(ctx, group)).To(Succeed())
					}

					Expect(k8sClient.Delete(ctx, group)).To(Succeed())
				}
			})
			It("should delete Network Resource", func() {
				Expect(k8sClient.Delete(ctx, ozresource)).To(Succeed())
				resourceDeleted := false
				mux.HandleFunc("/api/networks/test/resources/test", func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					if r.Method == http.MethodDelete {
						resourceDeleted = true
						_, err := w.Write([]byte(`{}`))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resourceDeleted).To(BeTrue())

				err = k8sClient.Get(ctx, typeNamespacedName, ozresource)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
			It("should remove resource cleanup finalizer from solely-owned NBGroups", func() {
				group := &openzrov1.NBGroup{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meow"}, group)
				if errors.IsNotFound(err) {
					return
				}
				Expect(group.Finalizers).To(BeEmpty())
			})
			It("should remove owner reference from shared NBGroups", func() {
				group := &openzrov1.NBGroup{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "meowdelete"}, group)).To(Succeed())
				Expect(group.Finalizers).To(HaveLen(1))
				Expect(group.OwnerReferences).To(HaveLen(1))
			})
			It("should remove policy reference", func() {
				policy := &openzrov1.NBPolicy{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test"}, policy)).To(Succeed())
				Expect(policy.Status.ManagedServiceList).NotTo(ContainElement("default/test-resource"))
			})
		})
	})
})
