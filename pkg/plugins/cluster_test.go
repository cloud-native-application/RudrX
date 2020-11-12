package plugins

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/api/types"
)

var _ = Describe("DefinitionFiles", func() {

	route := types.Capability{
		Name: "routes.test",
		Type: types.TypeTrait,
		Parameters: []types.Parameter{
			{
				Name:     "domain",
				Required: true,
				Default:  "",
				Type:     cue.StringKind,
			},
		},
		Description: "description not defined",
		CrdName:     "routes.test",
	}

	deployment := types.Capability{
		Name:        "deployments.testapps",
		Type:        types.TypeWorkload,
		CrdName:     "deployments.testapps",
		Description: "description not defined",
		Parameters: []types.Parameter{
			{
				Type: cue.ListKind,
				Name: "env",
			},
			{
				Name:     "image",
				Type:     cue.StringKind,
				Default:  "",
				Short:    "i",
				Required: true,
				Usage:    "Which image would you like to use for your service",
			},
			{
				Name:    "port",
				Type:    cue.IntKind,
				Short:   "p",
				Default: int64(8080),
				Usage:   "Which port do you want customer traffic sent to",
			},
		},
	}

	websvc := types.Capability{
		Name:        "webservice.testapps",
		Type:        types.TypeWorkload,
		Description: "description not defined",
		Parameters: []types.Parameter{{
			Name: "env", Type: cue.ListKind,
		}, {
			Name:     "image",
			Type:     cue.StringKind,
			Default:  "",
			Short:    "i",
			Required: true,
			Usage:    "Which image would you like to use for your service",
		}, {
			Name:    "port",
			Type:    cue.IntKind,
			Short:   "p",
			Default: int64(6379),
			Usage:   "Which port do you want customer traffic sent to",
		}},
		CrdName: "webservice.testapps",
	}

	req, _ := labels.NewRequirement("usecase", selection.Equals, []string{"forplugintest"})
	selector := labels.NewSelector().Add(*req)

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("gettrait", func() {
		traitDefs, _, err := GetTraitsFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting trait definitions %v", traitDefs))
		for i := range traitDefs {
			// CueTemplate should always be fulfilled, even those whose CueTemplateURI is assigend,
			By("check CueTemplate is fulfilled")
			Expect(traitDefs[i].CueTemplate).ShouldNot(BeEmpty())
			traitDefs[i].CueTemplate = ""
			traitDefs[i].DefinitionPath = ""
		}
		Expect(traitDefs).Should(Equal([]types.Capability{route}))
	})

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("getworkload", func() {
		workloadDefs, _, err := GetWorkloadsFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting workload definitions  %v", workloadDefs))
		for i := range workloadDefs {
			// CueTemplate should always be fulfilled, even those whose CueTemplateURI is assigend,
			By("check CueTemplate is fulfilled")
			Expect(workloadDefs[i].CueTemplate).ShouldNot(BeEmpty())
			workloadDefs[i].CueTemplate = ""
			workloadDefs[i].DefinitionPath = ""
		}
		Expect(workloadDefs).Should(Equal([]types.Capability{deployment, websvc}))
	})
	It("getall", func() {
		alldef, err := GetCapabilitiesFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting all definitions %v", alldef))
		for i := range alldef {
			alldef[i].CueTemplate = ""
			alldef[i].DefinitionPath = ""
		}
		Expect(alldef).Should(Equal([]types.Capability{deployment, websvc, route}))
	})

})
