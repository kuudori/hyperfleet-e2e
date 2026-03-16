package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // Gomega matchers are designed to be used with dot import
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: adapter][maestro-transport] Adapter Framework - Maestro Transportation Layer",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var clusterName string
		var resourceBundleID string
		var namespaceName string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
			// Create cluster for all tests in this suite
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
			Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
			clusterID = *cluster.Id
			clusterName = cluster.Name
			ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, clusterName)
		})

		ginkgo.Describe("Maestro Transport Happy Path", func() {
			// This test validates the complete Maestro transport happy path:
			// 1. Creating a cluster via HyperFleet API triggers adapter to create ManifestWork
			// 2. ManifestWork is created on Maestro server with correct metadata
			// 3. Maestro agent applies ManifestWork content to target cluster
			// 4. Adapter discovers ManifestWork via statusFeedback and reports status to API
			ginkgo.It("should create ManifestWork and report status via Maestro transport",
				func(ctx context.Context) {
					// Define variables for test adapter yaml
					adapterName := "cl-maestro"
					maestroConsumerName := "cluster1"
					namespaceName = fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)
					configmapName := fmt.Sprintf("%s-%s-configmap", clusterID, adapterName)

					ginkgo.By("Step 1: Verify cluster was created with generation=1")
					cluster, err := h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					Expect(cluster.Generation).To(Equal(int32(1)), "cluster should have generation=1")

					var resourceBundle *maestro.ResourceBundle

					ginkgo.By("Step 2: Verify ManifestWork (resource bundle) was created on Maestro")
					// Query Maestro API via HTTP client
					Eventually(func(g Gomega) {
						rb, err := h.GetMaestroClient().FindResourceBundleByClusterID(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to find resource bundle for cluster")
						resourceBundle = rb

						// Verify consumer name
						g.Expect(rb.ConsumerName).To(Equal(maestroConsumerName),
							"resource bundle should target correct consumer")

						// Verify version
						g.Expect(rb.Version).To(Equal(1),
							"resource bundle should have version=1")

						// Verify manifest names
						expectedManifests := []string{
							fmt.Sprintf("%s-%s-namespace", clusterID, adapterName),
							fmt.Sprintf("%s-%s-configmap", clusterID, adapterName),
						}
						g.Expect(rb.Manifests).To(HaveLen(2),
							"resource bundle should contain 2 manifests")

						manifestNames := make([]string, len(rb.Manifests))
						for i, m := range rb.Manifests {
							manifestNames[i] = m.Metadata.Name
						}
						g.Expect(manifestNames).To(ConsistOf(expectedManifests),
							"manifest names should match expected pattern")

						ginkgo.GinkgoWriter.Printf("Found resource bundle ID: %s\n", rb.ID)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 3: Verify ManifestWork metadata (labels and annotations)")
					Expect(resourceBundle).NotTo(BeNil(), "resource bundle should be found")

					// Record resource bundle ID for cleanup
					resourceBundleID = resourceBundle.ID

					// Verify labels
					Expect(resourceBundle.Metadata.Labels).To(HaveKey(client.KeyClusterID))
					Expect(resourceBundle.Metadata.Labels[client.KeyClusterID]).To(Equal(clusterID))
					Expect(resourceBundle.Metadata.Labels).To(HaveKey(client.KeyGeneration))
					Expect(resourceBundle.Metadata.Labels[client.KeyGeneration]).To(Equal("1"))
					Expect(resourceBundle.Metadata.Labels).To(HaveKey(client.KeyAdapter))
					Expect(resourceBundle.Metadata.Labels[client.KeyAdapter]).To(Equal(adapterName))

					// Verify annotations
					Expect(resourceBundle.Metadata.Annotations).To(HaveKey(client.KeyGeneration))
					Expect(resourceBundle.Metadata.Annotations[client.KeyGeneration]).To(Equal("1"))
					Expect(resourceBundle.Metadata.Annotations).To(HaveKey(client.KeyManagedBy))
					Expect(resourceBundle.Metadata.Annotations[client.KeyManagedBy]).To(Equal(adapterName))

					ginkgo.By("Step 4: Verify feedbackRules configuration in Maestro resource bundle")
					// Verify manifestConfigs exist
					Expect(resourceBundle.ManifestConfigs).NotTo(BeEmpty(), "manifestConfigs should be present")
					Expect(resourceBundle.ManifestConfigs).To(HaveLen(2), "should have 2 manifest configs")

					// Verify namespace feedbackRules
					namespaceFeedback := maestro.FindManifestConfig(resourceBundle.ManifestConfigs,
						maestro.ResourceIdentifier{
							Name:     namespaceName,
							Resource: "namespaces",
						})
					Expect(namespaceFeedback).NotTo(BeNil(), "namespace manifest config should exist")
					Expect(namespaceFeedback.FeedbackRules).NotTo(BeEmpty())

					// Verify configmap feedbackRules
					configmapFeedback := maestro.FindManifestConfig(resourceBundle.ManifestConfigs,
						maestro.ResourceIdentifier{
							Name:      configmapName,
							Resource:  "configmaps",
							Namespace: namespaceName,
						})
					Expect(configmapFeedback).NotTo(BeNil(), "configmap manifest config should exist")
					Expect(configmapFeedback.FeedbackRules).NotTo(BeEmpty())

					ginkgo.By("Step 5: Verify K8s resources created by Maestro agent on target cluster")
					// Wait for Maestro agent to apply the ManifestWork content

					Eventually(func(g Gomega) {
						// Verify Namespace exists, is Active, and has correct labels/annotations
						ns, err := h.GetNamespace(ctx, namespaceName)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get namespace")

						// Verify phase
						g.Expect(ns.Status.Phase).To(Equal(corev1.NamespaceActive), "namespace should be Active")

						// Verify labels
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/component"))
						g.Expect(ns.Labels["app.kubernetes.io/component"]).To(Equal("adapter-task-config"))
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/instance"))
						g.Expect(ns.Labels["app.kubernetes.io/instance"]).To(Equal(adapterName))
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/name"))
						g.Expect(ns.Labels["app.kubernetes.io/name"]).To(Equal("cl-maestro"))
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/transport"))
						g.Expect(ns.Labels["app.kubernetes.io/transport"]).To(Equal("maestro"))

						// Verify annotations
						g.Expect(ns.Annotations).To(HaveKey(client.KeyGeneration))
						g.Expect(ns.Annotations[client.KeyGeneration]).To(Equal("1"))

						// Verify ConfigMap exists with correct labels, annotations, and data
						cm, err := h.GetConfigMap(ctx, namespaceName, configmapName)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get configmap")

						// Verify labels
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/component"))
						g.Expect(cm.Labels["app.kubernetes.io/component"]).To(Equal("adapter-task-config"))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/instance"))
						g.Expect(cm.Labels["app.kubernetes.io/instance"]).To(Equal(adapterName))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/name"))
						g.Expect(cm.Labels["app.kubernetes.io/name"]).To(Equal("cl-maestro"))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/version"))
						g.Expect(cm.Labels["app.kubernetes.io/version"]).To(Equal("1.0.0"))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/transport"))
						g.Expect(cm.Labels["app.kubernetes.io/transport"]).To(Equal("maestro"))

						// Verify annotations
						g.Expect(cm.Annotations).To(HaveKey(client.KeyGeneration))
						g.Expect(cm.Annotations[client.KeyGeneration]).To(Equal("1"))

						// Verify data
						g.Expect(cm.Data).To(HaveKey("cluster_id"))
						g.Expect(cm.Data["cluster_id"]).To(Equal(clusterID))
						g.Expect(cm.Data).To(HaveKey("cluster_name"))
						g.Expect(cm.Data["cluster_name"]).To(Equal(clusterName))

						ginkgo.GinkgoWriter.Printf("Verified K8s resources created: namespace=%s, configmap=%s\n",
							namespaceName, configmapName)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 6: Verify adapter status report to HyperFleet API")
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						// Find the adapter status
						var adapterStatus *openapi.AdapterStatus
						for _, status := range statuses.Items {
							if status.Adapter == adapterName {
								adapterStatus = &status
								break
							}
						}
						g.Expect(adapterStatus).NotTo(BeNil(),
							"adapter %s should report status", adapterName)

						// Verify observed_generation
						g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
							"adapter should have observed_generation=1")

						// Verify observed_time is present
						g.Expect(adapterStatus.LastReportTime).NotTo(BeZero(),
							"adapter should have valid observed_time")

						// Verify conditions
						hasApplied := h.HasAdapterCondition(
							adapterStatus.Conditions,
							client.ConditionTypeApplied,
							openapi.AdapterConditionStatusTrue,
						)
						g.Expect(hasApplied).To(BeTrue(),
							"adapter should have Applied=True")

						// Check Applied condition reason
						appliedCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeApplied)
						g.Expect(appliedCond).NotTo(BeNil())
						g.Expect(appliedCond.Reason).NotTo(BeNil())
						g.Expect(*appliedCond.Reason).To(Equal("AppliedManifestWorkComplete"))

						hasAvailable := h.HasAdapterCondition(
							adapterStatus.Conditions,
							client.ConditionTypeAvailable,
							openapi.AdapterConditionStatusTrue,
						)
						g.Expect(hasAvailable).To(BeTrue(),
							"adapter should have Available=True")

						// Check Available condition reason
						availableCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeAvailable)
						g.Expect(availableCond).NotTo(BeNil())
						g.Expect(availableCond.Reason).NotTo(BeNil())
						g.Expect(*availableCond.Reason).To(Equal("AllResourcesAvailable"))

						hasHealth := h.HasAdapterCondition(
							adapterStatus.Conditions,
							client.ConditionTypeHealth,
							openapi.AdapterConditionStatusTrue,
						)
						g.Expect(hasHealth).To(BeTrue(),
							"adapter should have Health=True")

						// Check Health condition reason
						healthCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeHealth)
						g.Expect(healthCond).NotTo(BeNil())
						g.Expect(healthCond.Reason).NotTo(BeNil())
						g.Expect(*healthCond.Reason).To(Equal("Healthy"))

						// Verify data fields
						g.Expect(adapterStatus.Data).NotTo(BeNil(), "adapter data should be present")
						if adapterStatus.Data == nil {
							return // let Eventually retry with clean failure message
						}

						// Verify manifestwork data
						manifestworkData, ok := (*adapterStatus.Data)["manifestwork"].(map[string]interface{})
						g.Expect(ok).To(BeTrue(), "manifestwork data should be present")
						g.Expect(manifestworkData["name"]).To(Equal(fmt.Sprintf("%s-%s", clusterID, adapterName)))

						// Verify namespace data
						namespaceData, ok := (*adapterStatus.Data)["namespace"].(map[string]interface{})
						g.Expect(ok).To(BeTrue(), "namespace data should be present")
						g.Expect(namespaceData["phase"]).To(Equal("Active"))
						g.Expect(namespaceData["name"]).To(Equal(namespaceName))

						// Verify configmap data
						configmapData, ok := (*adapterStatus.Data)["configmap"].(map[string]interface{})
						g.Expect(ok).To(BeTrue(), "configmap data should be present")
						g.Expect(configmapData["clusterId"]).To(Equal(clusterID))
						g.Expect(configmapData["name"]).To(Equal(configmapName))

						if appliedCond != nil && appliedCond.Reason != nil &&
							availableCond != nil && availableCond.Reason != nil &&
							healthCond != nil && healthCond.Reason != nil {
							ginkgo.GinkgoWriter.Printf("Verified adapter status report: Applied=%s, Available=%s, Health=%s\n",
								*appliedCond.Reason, *availableCond.Reason, *healthCond.Reason)
						}
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
				})
		})

		ginkgo.Describe("Maestro Generation-based Idempotency", func() {
			// This test validates the generation-based idempotency mechanism:
			// 1. When a ManifestWork does not exist, it should be created
			// 2. When the same event is reprocessed with the same generation, the operation should be skipped
			// 3. The Maestro resource version should remain unchanged across multiple Skip operations
			ginkgo.It("should skip ManifestWork operation when generation is unchanged",
				func(ctx context.Context) {
					// Set namespace name for cleanup
					adapterName := "cl-maestro"
					namespaceName = fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)

					ginkgo.By("Step 1: Verify cluster was created with generation=1")
					cluster, err := h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					Expect(cluster.Generation).To(Equal(int32(1)), "cluster should have generation=1")

					var resourceBundle *maestro.ResourceBundle

					ginkgo.By("Step 2: Wait for initial ManifestWork creation and capture resource bundle ID")
					// Query Maestro API to find the resource bundle
					Eventually(func(g Gomega) {
						rb, err := h.GetMaestroClient().FindResourceBundleByClusterID(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to find resource bundle for cluster")
						resourceBundle = rb

						// Verify version is 1 (initial creation)
						g.Expect(rb.Version).To(Equal(1),
							"resource bundle should have version=1 after initial creation")

						ginkgo.GinkgoWriter.Printf("Found resource bundle ID: %s with version: %d\n", rb.ID, rb.Version)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					Expect(resourceBundle).NotTo(BeNil(), "resource bundle should be found")
					resourceBundleID = resourceBundle.ID
					initialVersion := resourceBundle.Version

					ginkgo.By("Step 3: Capture initial adapter status timestamp before skip period")
					// Capture the initial lastReportTime to verify adapter continues processing
					var initialReportTime time.Time
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						var adapterStatus *openapi.AdapterStatus
						for _, status := range statuses.Items {
							if status.Adapter == adapterName {
								adapterStatus = &status
								break
							}
						}
						g.Expect(adapterStatus).NotTo(BeNil(), "adapter status should exist")
						g.Expect(adapterStatus.LastReportTime).NotTo(BeZero(), "lastReportTime should be set")

						initialReportTime = adapterStatus.LastReportTime
						ginkgo.GinkgoWriter.Printf("Captured initial lastReportTime: %v\n", initialReportTime)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 4: Verify adapter continued processing during skip period")
					// We wait up to 3-4 polling cycles to ensure multiple processing cycles occur.
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						var adapterStatus *openapi.AdapterStatus
						for _, status := range statuses.Items {
							if status.Adapter == adapterName {
								adapterStatus = &status
								break
							}
						}
						g.Expect(adapterStatus).NotTo(BeNil(), "adapter status should exist")

						// Verify lastReportTime was updated (adapter is still processing)
						g.Expect(adapterStatus.LastReportTime.After(initialReportTime)).To(BeTrue(),
							"adapter should have updated lastReportTime, indicating it processed events during skip period")

						// Verify observedGeneration is still 1 (adapter is processing the same generation)
						g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
							"adapter should still observe generation 1")

						ginkgo.GinkgoWriter.Printf("Verified adapter processed events: lastReportTime updated from %v to %v\n",
							initialReportTime, adapterStatus.LastReportTime)
					}, 3*h.Cfg.Polling.Interval, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 5: Verify Maestro resource version does not change on Skip")
					// Query the resource bundle again to verify version remains unchanged
					Eventually(func(g Gomega) {
						rb, err := h.GetMaestroClient().FindResourceBundleByClusterID(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to find resource bundle")

						// Version should remain at initial version (1)
						g.Expect(rb.Version).To(Equal(initialVersion),
							"resource bundle version should remain unchanged across Skip operations")

						ginkgo.GinkgoWriter.Printf("Verified resource bundle version remains at: %d\n", rb.Version)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
				})
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			// Skip cleanup if helper not initialized
			if h == nil {
				return
			}

			// Note: It will be replaced by API DELETE once HyperFleet API supports DELETE operations for clusters resource type.

			// Delete resource bundle first - this triggers Maestro agent to clean up K8s resources
			// The agent will delete the namespace and configmap according to deleteOption.propagationPolicy
			if resourceBundleID != "" {
				ginkgo.By("deleting resource bundle " + resourceBundleID)
				ginkgo.GinkgoWriter.Printf("Deleting resource bundle ID: %s\n", resourceBundleID)
				err := h.GetMaestroClient().DeleteResourceBundle(ctx, resourceBundleID)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to delete resource bundle %s: %v\n", resourceBundleID, err)
				} else {
					ginkgo.GinkgoWriter.Printf("Successfully deleted resource bundle %s\n", resourceBundleID)
				}
			}

			// Delete namespace as a safety cleanup (in case Maestro agent didn't clean up)
			// This ensures the namespace is removed even if the agent deletion failed
			if namespaceName != "" {
				ginkgo.By("deleting namespace " + namespaceName)
				err := h.K8sClient.DeleteNamespaceAndWait(ctx, namespaceName)
				if err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to delete namespace %s: %v\n", namespaceName, err)
				} else {
					ginkgo.GinkgoWriter.Printf("Successfully deleted namespace %s\n", namespaceName)
				}
			}
		})
	},
)
