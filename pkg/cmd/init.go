package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"

	oamv1 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/kube"
)

var (
	settings = cli.New()
)

const initDesc = `
This command installs oam-kubernetes-runtime  onto your Kubernetes Cluster.
As with the rest of the RudrX commands, 'rudrx init' discovers Kubernetes clusters
by reading $KUBECONFIG (default '~/.kube/config') and using the default context.
When installing oam-kubernetes-runtime, 'rudrx init' will attempt to install the latest released
version. 
`

type initCmd struct {
	namespace string
	out       io.Writer
	client    client.Client
	config    *rest.Config
	version   string
}

const (
	DefaultOAMNS          = "oam-system"
	DefaultOAMReleaseName = "core-runtime"
	DefaultOAMChartName   = "crossplane-master/oam-kubernetes-runtime"
	DefaultOAMRepoName    = "crossplane-master"
	DefaultOAMRepoUrl     = "https://charts.crossplane.io/master"
	DefaultOAMVersion     = ">0.0.0-0"
)

var (
	defaultObject = []interface{}{
		&oamv1.WorkloadDefinition{},
		&oamv1.ApplicationConfiguration{},
		&oamv1.Component{},
		&oamv1.TraitDefinition{},
		&oamv1.ContainerizedWorkload{},
		&oamv1.HealthScope{},
		&oamv1.ManualScalerTrait{},
		&oamv1.ScopeDefinition{},
	}

	workloadResource = map[string]string{
		"statefulset": "statefulsets.apps",
		"daemonset":   "daemonsets.apps",
		"deployment":  "deployments.apps",
		"job":         "jobs.batch",
		"secret":      "secrets",
		"service":     "services",
		"configmap":   "configmaps",
	}
)

func NewInitCommand(f cmdutil.Factory, c client.Client, ioStreams cmdutil.IOStreams) *cobra.Command {

	i := &initCmd{out: ioStreams.Out}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize RudrX on both client and server",
		Long:  initDesc,
		RunE: func(cmd *cobra.Command, args []string) error {
			i.client = c
			i.namespace = DefaultOAMNS
			return i.run(ioStreams)
		},
	}

	flag := cmd.Flags()
	flag.StringVarP(&i.version, "version", "v", "", "Override chart version")

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams) error {

	if err := cmdutil.GetKubeClient(); err != nil {
		return fmt.Errorf("could not get kubernetes client: %s", err)
	}

	if !cmdutil.IsNamespaceExist(i.client, DefaultOAMNS) {
		if err := cmdutil.NewNamespace(i.client, DefaultOAMNS); err != nil {
			return err
		}
	}

	if i.IsOamRuntimeExist() {
		fmt.Println("Successfully initialized.")
		return nil
	}

	if err := InstallOamRuntime(ioStreams, i.version); err != nil {
		return err
	}

	if err := GenNativeResourceDefinition(i.client); err != nil {
		return err
	}

	return nil
}

func (i *initCmd) IsOamRuntimeExist() bool {

	for _, object := range defaultObject {
		if err := cmdutil.IsCoreCRDExist(i.client, context.Background(), object.(runtime.Object)); err != nil {
			return false
		}
	}
	return true
}

func InstallOamRuntime(ioStreams cmdutil.IOStreams, version string) error {

	if !IsHelmRepositoryExist(DefaultOAMRepoName, DefaultOAMRepoUrl) {
		err := AddHelmRepository(DefaultOAMRepoName, DefaultOAMRepoUrl,
			"", "", "", "", "", false, ioStreams.Out)
		if err != nil {
			return err
		}
	}

	chartClient, err := NewHelmInstall(version, ioStreams)
	if err != nil {
		return err
	}

	chartRequested, err := GetChart(chartClient, DefaultOAMChartName)
	if err != nil {
		return err
	}

	release, err := chartClient.Run(chartRequested, nil)
	if err != nil {
		return err
	}

	fmt.Println("Successfully installed oam-kubernetes-runtime release: ", release.Name)
	return nil
}

func NewHelmInstall(version string, ioStreams cmdutil.IOStreams) (*action.Install, error) {
	actionConfig := new(action.Configuration)

	if err := actionConfig.Init(
		kube.GetConfig(cmdutil.GetKubeConfig(), "", DefaultOAMNS),
		DefaultOAMNS,
		os.Getenv("HELM_DRIVER"),
		ioStreams.Infof,
	); err != nil {
		return nil, err
	}

	client := action.NewInstall(actionConfig)
	client.Namespace = DefaultOAMNS
	client.ReleaseName = DefaultOAMReleaseName

	if len(version) > 0 {
		client.Version = version
		return client, nil
	}
	client.Version = DefaultOAMVersion
	return client, nil
}

func GetChart(client *action.Install, name string) (*chart.Chart, error) {
	settings.Debug = true

	chartPath, err := client.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		return nil, err
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return nil, err
	}
	return chartRequested, nil
}

func AddHelmRepository(name, url, username, password, certFile, keyFile, caFile string, insecureSkipTLSverify bool, out io.Writer) error {
	var f repo.File
	c := repo.Entry{
		Name:                  name,
		URL:                   url,
		Username:              username,
		Password:              password,
		CertFile:              certFile,
		KeyFile:               keyFile,
		CAFile:                caFile,
		InsecureSkipTLSverify: insecureSkipTLSverify,
	}

	r, err := repo.NewChartRepository(&c, getter.All(settings))
	if err != nil {
		return err
	}

	if _, err := r.DownloadIndexFile(); err != nil {
		return errors.Wrapf(err, "looks like %q is not a valid chart repository or cannot be reached", url)
	}

	f.Update(&c)

	if err := f.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return err
	}
	fmt.Fprintf(out, "%q has been added to your repositories\n", name)
	return nil
}

func IsHelmRepositoryExist(name, url string) bool {
	repos := GetHelmRepositoryList()
	for _, repo := range repos {
		if repo.Name == name && repo.URL == url {
			return true
		}
	}
	return false
}

func GetHelmRepositoryList() []*repo.Entry {
	f, err := repo.LoadFile(settings.RepositoryConfig)
	if err == nil && len(f.Repositories) > 0 {
		return filterRepos(f.Repositories)
	}
	return nil
}

func filterRepos(repos []*repo.Entry) []*repo.Entry {
	filteredRepos := make([]*repo.Entry, 0)
	for _, repo := range repos {
		filteredRepos = append(filteredRepos, repo)
	}
	return filteredRepos
}

func GenNativeResourceDefinition(c client.Client) error {
	for name, reference := range workloadResource {
		workload := NewWorkloadDefinition(name, reference)
		err := c.Get(context.Background(), client.ObjectKey{Name: name}, workload)
		if kubeerrors.IsNotFound(err) {
			if err := c.Create(context.Background(), workload); err != nil {
				return fmt.Errorf("create workload definition %s hit an issue: %v", reference, err)
			}
		} else if err != nil {
			return fmt.Errorf("get workload definition hit an issue: %v", err)
		}
	}

	return nil

}

func NewWorkloadDefinition(name, reference string) *oamv1.WorkloadDefinition {
	return &oamv1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: oamv1.WorkloadDefinitionSpec{
			Reference: oamv1.DefinitionReference{Name: reference},
		},
	}
}
