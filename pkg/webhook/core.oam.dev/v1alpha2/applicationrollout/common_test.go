/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package applicationrollout

import (
	"sort"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("Application Deployment Common Function Test", func() {
	BeforeEach(func() {

	})

	Context("Test Find Common Component Function", func() {
		var targetApp, sourceApp *v1alpha2.ApplicationConfiguration

		BeforeEach(func() {
			targetApp = &v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{},
				},
			}
			sourceApp = &v1alpha2.ApplicationConfiguration{
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{},
				},
			}
		})

		It("Test source app is nil", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			common := FindCommonComponent(targetApp, nil)
			Expect(common).Should(BeEquivalentTo([]string{"a", "b", "c"}))
		})

		It("Test has one component", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c"}))
		})

		It("Test has one common components", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"d", "c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c"}))
		})

		It("Test has more than 1 common component", func() {
			fillApplication(&targetApp.Spec, []string{"b", "a", "c"})
			fillApplication(&sourceApp.Spec, []string{"c", "b"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c", "b"}))
		})

		It("Test has more than 1 common component", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"d", "e", "c", "a"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c", "a"}))
		})
	})
})

func fillApplication(app *v1alpha2.ApplicationConfigurationSpec, componentNames []string) {
	for _, name := range componentNames {
		app.Components = append(app.Components, v1alpha2.ApplicationConfigurationComponent{
			RevisionName: utils.ConstructRevisionName(name, 1),
		})
	}
}

var _ = Describe("Test find common component func", func() {
	It("Test source app is nil", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		common := FindCommonComponentWithManifest(target, nil)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"a", "b", "c"}))
	})

	It("Test has one component", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		source := fillWorkloads([]string{"c"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"c"}))
	})

	It("Test has one common components", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		source := fillWorkloads([]string{"d", "c"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"c"}))
	})

	It("Test has more than 1 common component", func() {
		target := fillWorkloads([]string{"b", "a", "c"})
		source := fillWorkloads([]string{"c", "b"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"b", "c"}))
	})

	It("Test has more than 1 common component", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		source := fillWorkloads([]string{"d", "e", "c", "a"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"a", "c"}))
	})
})

func fillWorkloads(componentNames []string) map[string]*unstructured.Unstructured {
	w := make(map[string]*unstructured.Unstructured)
	for _, s := range componentNames {
		// we don't need real workload
		w[s] = nil
	}
	return w
}
