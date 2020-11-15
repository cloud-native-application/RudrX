package oam

import (
	"fmt"
	"os"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/api/v1alpha1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	Scheme = k8sruntime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = certmanager.AddToScheme(Scheme)
	_ = core.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
	// +kubebuilder:scaffold:scheme
}

func InitArgs() (types.Args, error) {
	restConf, err := config.GetConfig()
	if err != nil {
		fmt.Println("get kubeConfig err", err)
		os.Exit(1)
	}

	return types.Args{
		Config: restConf,
		Schema: Scheme,
	}, nil
}
