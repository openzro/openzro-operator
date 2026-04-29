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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openzrov1 "github.com/openzro/openzro-operator/api/v1"
	"github.com/openzro/openzro-operator/internal/util"
	openzro "github.com/openzro/openzro/management/client/rest"
	"github.com/openzro/openzro/management/server/http/api"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("OZPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		var resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		ozpolicy := &openzrov1.OZPolicy{}
		var openzroClient *openzro.Client
		var mux *http.ServeMux
		var server *httptest.Server

		BeforeEach(func() {
			ctrl.SetLogger(logr.New(GinkgoLogr.GetSink()))
			mux = &http.ServeMux{}
			server = httptest.NewServer(mux)
			openzroClient = openzro.New(server.URL, "ABC")

			By("creating the custom resource for the Kind OZPolicy")
			err := k8sClient.Get(ctx, typeNamespacedName, ozpolicy)
			if err != nil && errors.IsNotFound(err) {
				resource := &openzrov1.OZPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:       resourceName,
						Finalizers: []string{"openzro.io/cleanup"},
					},
					Spec: openzrov1.OZPolicySpec{
						Name:          "Test",
						SourceGroups:  []string{"All"},
						Bidirectional: true,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
				ozpolicy = resource
			}
		})

		AfterEach(func() {
			resource := &openzrov1.OZPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if !errors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())

				if len(resource.Finalizers) > 0 {
					resource.Finalizers = nil
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}

				By("Cleanup the specific resource instance OZPolicy")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			ozresource := &openzrov1.OZResource{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test"}, ozresource)
			if !errors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())

				By("Cleanup the specific resource instance OZResource")
				Expect(k8sClient.Delete(ctx, ozresource)).To(Succeed())
			}
		})
		When("Not enough information to create policy", func() {
			It("should not create any policy", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
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
			})
		})

		When("Enough information to create TCP policy", func() {
			It("should create 1 policy", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				nbResource := &openzrov1.OZResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: openzrov1.OZResourceSpec{
						Name:       "meow",
						Groups:     []string{"test"},
						NetworkID:  "test",
						Address:    "test.default.svc.cluster.local",
						PolicyName: resourceName,
						TCPPorts:   []int32{443},
					},
				}
				Expect(k8sClient.Create(ctx, nbResource)).To(Succeed())

				nbResource.Status = openzrov1.OZResourceStatus{
					TCPPorts:   []int32{443},
					PolicyName: &resourceName,
					Groups:     []string{"test"},
				}
				Expect(k8sClient.Status().Update(ctx, nbResource)).To(Succeed())

				ozpolicy.Status.ManagedServiceList = append(ozpolicy.Status.ManagedServiceList, "default/test")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				policyCreated := false
				mux.HandleFunc("/api/policies", func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					if r.Method == http.MethodPost {
						var policyReq api.PostApiPoliciesJSONRequestBody
						bs, err := io.ReadAll(r.Body)
						Expect(err).NotTo(HaveOccurred())
						err = json.Unmarshal(bs, &policyReq)
						Expect(err).NotTo(HaveOccurred())
						Expect(policyReq.Name).To(Equal("Test TCP"))
						Expect(policyReq.Description).To(Or(BeNil(), BeEquivalentTo(util.Ptr(""))))
						Expect(policyReq.Enabled).To(BeTrue())
						Expect(policyReq.SourcePostureChecks).To(BeNil())
						Expect(policyReq.Rules).To(HaveLen(1))
						Expect(policyReq.Rules[0].Action).To(BeEquivalentTo(api.PolicyRuleActionAccept))
						Expect(policyReq.Rules[0].Bidirectional).To(BeTrue())
						Expect(policyReq.Rules[0].Description).To(Or(BeNil(), BeEquivalentTo(util.Ptr(""))))
						Expect(policyReq.Rules[0].DestinationResource).To(BeNil())
						Expect(policyReq.Rules[0].Destinations).NotTo(BeNil())
						Expect(*policyReq.Rules[0].Destinations).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Destinations)[0]).To(Equal("test"))
						Expect(policyReq.Rules[0].Enabled).To(BeTrue())
						Expect(policyReq.Rules[0].Name).To(Equal("Test TCP"))
						Expect(policyReq.Rules[0].Ports).NotTo(BeNil())
						Expect((*policyReq.Rules[0].Ports)).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Ports)[0]).To(Equal("443"))
						Expect(policyReq.Rules[0].Protocol).To(BeEquivalentTo(api.PolicyRuleProtocolTcp))
						Expect(policyReq.Rules[0].SourceResource).To(BeNil())
						Expect(policyReq.Rules[0].Sources).NotTo(BeNil())
						Expect(*policyReq.Rules[0].Sources).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Sources)[0]).To(Equal("meow"))

						policyCreated = true
						resp := api.Policy{
							Id: &resourceName,
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
				Expect(policyCreated).To(BeTrue())
			})
		})

		When("TCP information no longer sufficient", func() {
			It("should delete tcp policy", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				ozpolicy.Status.ManagedServiceList = append(ozpolicy.Status.ManagedServiceList, "default/noexist")
				ozpolicy.Status.TCPPolicyID = util.Ptr("policyid")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				policyDeleted := false
				mux.HandleFunc("/api/policies/policyid", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						policyDeleted = true
						_, err := w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(policyDeleted).To(BeTrue())
			})
		})

		When("Enough information to create UDP policy", func() {
			It("should create 1 policy", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				nbResource := &openzrov1.OZResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: openzrov1.OZResourceSpec{
						Name:       "meow",
						Groups:     []string{"test"},
						NetworkID:  "test",
						Address:    "test.default.svc.cluster.local",
						PolicyName: resourceName,
						UDPPorts:   []int32{443},
					},
				}
				Expect(k8sClient.Create(ctx, nbResource)).To(Succeed())

				nbResource.Status = openzrov1.OZResourceStatus{
					UDPPorts:   []int32{443},
					PolicyName: &resourceName,
					Groups:     []string{"test"},
				}
				Expect(k8sClient.Status().Update(ctx, nbResource)).To(Succeed())

				ozpolicy.Status.ManagedServiceList = append(ozpolicy.Status.ManagedServiceList, "default/test")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				policyCreated := false
				mux.HandleFunc("/api/policies", func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					if r.Method == http.MethodPost {
						var policyReq api.PostApiPoliciesJSONRequestBody
						bs, err := io.ReadAll(r.Body)
						Expect(err).NotTo(HaveOccurred())
						err = json.Unmarshal(bs, &policyReq)
						Expect(err).NotTo(HaveOccurred())
						Expect(policyReq.Name).To(Equal("Test UDP"))
						Expect(policyReq.Description).To(Or(BeNil(), BeEquivalentTo(util.Ptr(""))))
						Expect(policyReq.Enabled).To(BeTrue())
						Expect(policyReq.SourcePostureChecks).To(BeNil())
						Expect(policyReq.Rules).To(HaveLen(1))
						Expect(policyReq.Rules[0].Action).To(BeEquivalentTo(api.PolicyRuleActionAccept))
						Expect(policyReq.Rules[0].Bidirectional).To(BeTrue())
						Expect(policyReq.Rules[0].Description).To(Or(BeNil(), BeEquivalentTo(util.Ptr(""))))
						Expect(policyReq.Rules[0].DestinationResource).To(BeNil())
						Expect(policyReq.Rules[0].Destinations).NotTo(BeNil())
						Expect(*policyReq.Rules[0].Destinations).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Destinations)[0]).To(Equal("test"))
						Expect(policyReq.Rules[0].Enabled).To(BeTrue())
						Expect(policyReq.Rules[0].Name).To(Equal("Test UDP"))
						Expect(policyReq.Rules[0].Ports).NotTo(BeNil())
						Expect((*policyReq.Rules[0].Ports)).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Ports)[0]).To(Equal("443"))
						Expect(policyReq.Rules[0].Protocol).To(BeEquivalentTo(api.PolicyRuleProtocolUdp))
						Expect(policyReq.Rules[0].SourceResource).To(BeNil())
						Expect(policyReq.Rules[0].Sources).NotTo(BeNil())
						Expect(*policyReq.Rules[0].Sources).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Sources)[0]).To(Equal("meow"))

						policyCreated = true
						resp := api.Policy{
							Id: &resourceName,
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
				Expect(policyCreated).To(BeTrue())
			})
		})

		When("UDP information no longer sufficient", func() {
			It("should delete udp policy", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				ozpolicy.Status.ManagedServiceList = append(ozpolicy.Status.ManagedServiceList, "default/noexist")
				ozpolicy.Status.UDPPolicyID = util.Ptr("policyid")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				policyDeleted := false
				mux.HandleFunc("/api/policies/policyid", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						policyDeleted = true
						_, err := w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(policyDeleted).To(BeTrue())
			})
		})

		When("Existing protocol gets restricted", func() {
			It("Should delete protocol policy", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				nbResource := &openzrov1.OZResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: openzrov1.OZResourceSpec{
						Name:       "meow",
						Groups:     []string{"test"},
						NetworkID:  "test",
						Address:    "test.default.svc.cluster.local",
						PolicyName: resourceName,
						TCPPorts:   []int32{443},
					},
				}
				Expect(k8sClient.Create(ctx, nbResource)).To(Succeed())

				nbResource.Status = openzrov1.OZResourceStatus{
					TCPPorts:   []int32{443},
					PolicyName: &resourceName,
					Groups:     []string{"test"},
				}
				Expect(k8sClient.Status().Update(ctx, nbResource)).To(Succeed())

				ozpolicy.Spec.Protocols = []string{"udp"}
				Expect(k8sClient.Update(ctx, ozpolicy)).To(Succeed())

				ozpolicy.Status.ManagedServiceList = append(ozpolicy.Status.ManagedServiceList, "default/test")
				ozpolicy.Status.TCPPolicyID = util.Ptr("policyid")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				policyDeleted := false
				mux.HandleFunc("/api/policies/policyid", func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					if r.Method == http.MethodDelete {
						policyDeleted = true
						_, err := w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(policyDeleted).To(BeTrue())
			})
		})

		When("Updating existing policy", func() {
			AfterEach(func() {
				ozresource := &openzrov1.OZResource{}
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-b"}, ozresource)
				if !errors.IsNotFound(err) {
					Expect(err).NotTo(HaveOccurred())

					By("Cleanup the specific resource instance OZResource")
					Expect(k8sClient.Delete(ctx, ozresource)).To(Succeed())
				}
			})

			It("Should give all information to Update method", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				nbResource := &openzrov1.OZResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: openzrov1.OZResourceSpec{
						Name:       "meow",
						Groups:     []string{"test"},
						NetworkID:  "test",
						Address:    "test.default.svc.cluster.local",
						PolicyName: resourceName,
						TCPPorts:   []int32{443},
					},
				}
				Expect(k8sClient.Create(ctx, nbResource)).To(Succeed())

				nbResource.Status = openzrov1.OZResourceStatus{
					TCPPorts:   []int32{443},
					PolicyName: &resourceName,
					Groups:     []string{"test"},
				}
				Expect(k8sClient.Status().Update(ctx, nbResource)).To(Succeed())

				nbResourceB := &openzrov1.OZResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-b",
						Namespace: "default",
					},
					Spec: openzrov1.OZResourceSpec{
						Name:       "meow-b",
						Groups:     []string{"test-b"},
						NetworkID:  "test",
						Address:    "test-b.default.svc.cluster.local",
						PolicyName: resourceName,
						TCPPorts:   []int32{80},
					},
				}
				Expect(k8sClient.Create(ctx, nbResourceB)).To(Succeed())

				nbResourceB.Status = openzrov1.OZResourceStatus{
					TCPPorts:   []int32{80},
					PolicyName: &resourceName,
					Groups:     []string{"test-b"},
				}
				Expect(k8sClient.Status().Update(ctx, nbResourceB)).To(Succeed())

				ozpolicy.Status.ManagedServiceList = append(ozpolicy.Status.ManagedServiceList, "default/test", "default/test-b")
				ozpolicy.Status.TCPPolicyID = util.Ptr("policyid")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				mux.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
					resp := []api.Group{
						{
							Id:   "meow",
							Name: "All",
						},
					}
					bs, err := json.Marshal(resp)
					Expect(err).NotTo(HaveOccurred())
					_, err = w.Write(bs)
					Expect(err).NotTo(HaveOccurred())
				})

				policyUpdated := false
				mux.HandleFunc("/api/policies/policyid", func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					if r.Method == http.MethodPut {
						policyUpdated = true

						var policyReq api.PostApiPoliciesJSONRequestBody
						bs, err := io.ReadAll(r.Body)
						Expect(err).NotTo(HaveOccurred())
						err = json.Unmarshal(bs, &policyReq)
						Expect(err).NotTo(HaveOccurred())
						Expect(policyReq.Name).To(Equal("Test TCP"))
						Expect(policyReq.Description).To(Or(BeNil(), BeEquivalentTo(util.Ptr(""))))
						Expect(policyReq.Enabled).To(BeTrue())
						Expect(policyReq.SourcePostureChecks).To(BeNil())
						Expect(policyReq.Rules).To(HaveLen(1))
						Expect(policyReq.Rules[0].Action).To(BeEquivalentTo(api.PolicyRuleActionAccept))
						Expect(policyReq.Rules[0].Bidirectional).To(BeTrue())
						Expect(policyReq.Rules[0].Description).To(Or(BeNil(), BeEquivalentTo(util.Ptr(""))))
						Expect(policyReq.Rules[0].DestinationResource).To(BeNil())
						Expect(policyReq.Rules[0].Destinations).NotTo(BeNil())
						Expect(*policyReq.Rules[0].Destinations).To(HaveLen(2))
						Expect((*policyReq.Rules[0].Destinations)).To(ConsistOf([]string{"test", "test-b"}))
						Expect(policyReq.Rules[0].Enabled).To(BeTrue())
						Expect(policyReq.Rules[0].Name).To(Equal("Test TCP"))
						Expect(policyReq.Rules[0].Ports).NotTo(BeNil())
						Expect((*policyReq.Rules[0].Ports)).To(HaveLen(2))
						Expect((*policyReq.Rules[0].Ports)).To(ConsistOf([]string{"443", "80"}))
						Expect(policyReq.Rules[0].Protocol).To(BeEquivalentTo(api.PolicyRuleProtocolTcp))
						Expect(policyReq.Rules[0].SourceResource).To(BeNil())
						Expect(policyReq.Rules[0].Sources).NotTo(BeNil())
						Expect(*policyReq.Rules[0].Sources).To(HaveLen(1))
						Expect((*policyReq.Rules[0].Sources)[0]).To(Equal("meow"))

						_, err = w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(policyUpdated).To(BeTrue())
			})
		})

		When("OZPolicy is set for deletion", func() {
			It("should delete Policies", func() {
				controllerReconciler := &OZPolicyReconciler{
					Client:  k8sClient,
					OpenZro: openzroClient,
				}

				ozpolicy.Status.TCPPolicyID = util.Ptr("policyidtcp")
				ozpolicy.Status.UDPPolicyID = util.Ptr("policyidudp")
				Expect(k8sClient.Status().Update(ctx, ozpolicy)).To(Succeed())

				Expect(k8sClient.Delete(ctx, ozpolicy)).To(Succeed())

				tcpPolicyDeleted := false
				mux.HandleFunc("/api/policies/policyidtcp", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						tcpPolicyDeleted = true
						_, err := w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				udpPolicyDeleted := false
				mux.HandleFunc("/api/policies/policyidudp", func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						udpPolicyDeleted = true
						_, err := w.Write([]byte("{}"))
						Expect(err).NotTo(HaveOccurred())
					}
				})

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(tcpPolicyDeleted).To(BeTrue())
				Expect(udpPolicyDeleted).To(BeTrue())

				err = k8sClient.Get(ctx, typeNamespacedName, ozpolicy)
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})
	})
})
