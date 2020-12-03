package common

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"

	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
)

var (
	// Scheme defines the default KubeVela schema
	Scheme = k8sruntime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = certmanager.AddToScheme(Scheme)
	_ = core.AddToScheme(Scheme)
	_ = v1alpha1.AddToScheme(Scheme)
	// +kubebuilder:scaffold:scheme
}

// InitBaseRestConfig will return reset config for create controller runtime client
func InitBaseRestConfig() (types.Args, error) {
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

// HTTPGet will send GET http request with context
func HTTPGet(ctx context.Context, url string) ([]byte, error) {
	// Change NewRequest to NewRequestWithContext and pass context it
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// RealtimePrintCommandOutput prints command output in real time
func RealtimePrintCommandOutput(cmd *exec.Cmd) error {
	writer := io.MultiWriter(os.Stdout)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
