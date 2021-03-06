package integration_test

import (
	"context"
	"path"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/registration/pkg/helpers"
	"github.com/open-cluster-management/registration/pkg/spoke"
	"github.com/open-cluster-management/registration/test/integration/util"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
)

var _ = ginkgo.Describe("Joining Process", func() {
	ginkgo.It("spokecluster should join successfully", func() {
		var err error

		spokeClusterName := "joiningtest-spokecluster"
		hubKubeconfigSecret := "joiningtest-hub-kubeconfig-secret"
		hubKubeconfigDir := path.Join(util.TestDir, "joiningtest", "hub-kubeconfig")

		// run registration agent
		go func() {
			agentOptions := spoke.SpokeAgentOptions{
				ClusterName:         spokeClusterName,
				BootstrapKubeconfig: bootstrapKubeConfigFile,
				HubKubeconfigSecret: hubKubeconfigSecret,
				HubKubeconfigDir:    hubKubeconfigDir,
			}
			err := agentOptions.RunSpokeAgent(context.Background(), &controllercmd.ControllerContext{
				KubeConfig:    spokeCfg,
				EventRecorder: util.NewIntegrationTestEventRecorder("joiningtest"),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}()

		// the spoke cluster and csr should be created after bootstrap
		gomega.Eventually(func() bool {
			if _, err := util.GetSpokeCluster(clusterClient, spokeClusterName); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		gomega.Eventually(func() bool {
			if _, err := util.FindUnapprovedSpokeCSR(kubeClient, spokeClusterName); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// the spoke cluster should has finalizer that is added by hub controller
		gomega.Eventually(func() bool {
			spokeCluster, err := util.GetSpokeCluster(clusterClient, spokeClusterName)
			if err != nil {
				return false
			}
			if len(spokeCluster.Finalizers) != 1 ||
				spokeCluster.Finalizers[0] != "cluster.open-cluster-management.io/api-resource-cleanup" {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// simulate hub cluster admin to accept the spokecluster and approve the csr
		err = util.AcceptSpokeCluster(clusterClient, spokeClusterName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		err = util.ApproveSpokeClusterCSR(kubeClient, spokeClusterName, time.Hour*24)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// the spoke cluster should have accepted condition after it is accepted
		gomega.Eventually(func() bool {
			spokeCluster, err := util.GetSpokeCluster(clusterClient, spokeClusterName)
			if err != nil {
				return false
			}
			accpeted := helpers.FindSpokeClusterCondition(spokeCluster.Status.Conditions, clusterv1.SpokeClusterConditionHubAccepted)
			if accpeted == nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// the hub kubeconfig secret should be filled after the csr is approved
		gomega.Eventually(func() bool {
			if _, err := util.GetFilledHubKubeConfigSecret(kubeClient, testNamespace, hubKubeconfigSecret); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())

		// simulate k8s to mount the hub kubeconfig secret
		err = util.MountHubKubeConfigs(kubeClient, hubKubeconfigDir, testNamespace, hubKubeconfigSecret)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// the spoke cluster should have joined condition finally
		gomega.Eventually(func() bool {
			spokeCluster, err := util.GetSpokeCluster(clusterClient, spokeClusterName)
			if err != nil {
				return false
			}
			joined := helpers.FindSpokeClusterCondition(spokeCluster.Status.Conditions, clusterv1.SpokeClusterConditionJoined)
			if joined == nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
	})
})
