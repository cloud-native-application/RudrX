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

package workloads

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	oamCore "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx = context.Background()

//var controllerDone chan struct{}
var mgr manager.Manager

func TestRollout(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rollout Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../../../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
		UseExistingCluster: &useExistCluster,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = oamCore.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	By("Starting the controller in the background")
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0",
		Port:               48081,
	})
	Expect(err).ToNot(HaveOccurred())

	//controllerDone = make(chan struct{}, 1)
	go func() {
		defer GinkgoRecover()
		//Expect(mgr.Start(controllerDone)).ToNot(HaveOccurred())
	}()

	close(done)
}, 60)

var _ = AfterSuite(func() {
	//By("Stop the controller")
	//close(controllerDone)

	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
