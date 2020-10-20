package e2e

import (
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/e2e"
)

var (
	envName                   = "env-trait"
	applicationName           = "app-trait-basic"
	applicationNotExistedName = "app-trait-basic-NOT-EXISTED"
	traitAlias                = "scale"
)

var _ = ginkgo.Describe("Trait", func() {
	e2e.RefreshContext("refresh")
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.WorkloadRunContext("deploy", fmt.Sprintf("vela comp deploy -t webservice %s -p 80 --image nginx:1.9.4", applicationName))

	e2e.TraitManualScalerAttachContext("vela attach trait", traitAlias, applicationName)

	// Trait
	ginkgo.Context("vela attach trait to a not existed app", func() {
		ginkgo.It("should print successful attached information", func() {
			cli := fmt.Sprintf("vela %s %s", traitAlias, applicationNotExistedName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("component name (" + applicationNotExistedName + ") doesn't exist"))
		})
	})

	//ginkgo.Context("vela detach trait", func() {
	//	ginkgo.It("should print successful detached information", func() {
	//		cli := fmt.Sprintf("vela %s --detach %s", traitAlias, applicationName)
	//		output, err := e2e.Exec(cli)
	//		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	//		expectedSubStr := fmt.Sprintf("Detaching %s from app %s", traitAlias, applicationName)
	//		gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr))
	//		gomega.Expect(output).To(gomega.ContainSubstring("Succeeded!"))
	//	})
	//})

	e2e.WorkloadDeleteContext("delete", applicationName)
})
