package integration_test

import (
	"context"
	"path"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/open-cluster-management/registration/pkg/spoke"
	"github.com/open-cluster-management/registration/test/integration/util"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
)

var _ = ginkgo.Describe("Certificate Rotation", func() {
	ginkgo.It("Certificate should be automatically rotated when it is about to expire", func() {
		var err error

		spokeClusterName := "rotationtest-spokecluster"
		hubKubeconfigSecret := "rotationtest-hub-kubeconfig-secret"
		hubKubeconfigDir := path.Join(util.TestDir, "rotationtest", "hub-kubeconfig")

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
				EventRecorder: util.NewIntegrationTestEventRecorder("rotationtest"),
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}()

		// after bootstrap the spokecluster and csr should be created
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

		// simulate hub cluster admin approve the csr with a short time certificate
		err = util.ApproveSpokeClusterCSR(kubeClient, spokeClusterName, time.Second*20)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// simulate hub cluster admin accept the spokecluster
		err = util.AcceptSpokeCluster(clusterClient, spokeClusterName)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

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

		// the agent should rotate the certificate because the certificate with a short valid time
		// the hub controller should auto approve it
		gomega.Eventually(func() bool {
			if _, err := util.FindAutoApprovedSpokeCSR(kubeClient, spokeClusterName); err != nil {
				return false
			}
			return true
		}, eventuallyTimeout, eventuallyInterval).Should(gomega.BeTrue())
	})
})
