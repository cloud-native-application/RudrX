package commands

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/openservicemesh/osm/pkg/cli"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
)

type VelaRuntimeStatus int

const (
	NotFound VelaRuntimeStatus = iota
	Pending
	Ready
	Error
)

type initCmd struct {
	namespace string
	ioStreams cmdutil.IOStreams
	client    client.Client
	chartPath string
	chartArgs chartArgs
	waitReady bool
}

type chartArgs struct {
	imageRepo       string
	imageTag        string
	imagePullPolicy string
}

type infoCmd struct {
	out io.Writer
}

func SystemCommandGroup(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "system management utilities",
		Long:  "system management utilities",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(NewAdminInfoCommand(ioStream), NewRefreshCommand(c, ioStream))
	return cmd
}

func NewAdminInfoCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &infoCmd{out: ioStreams.Out}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "show vela client and cluster chartPath",
		Long:  "show vela client and cluster chartPath",
		RunE: func(cmd *cobra.Command, args []string) error {
			return i.run(ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	return cmd
}

func (i *infoCmd) run(ioStreams cmdutil.IOStreams) error {
	clusterVersion, err := GetOAMReleaseVersion()
	if err != nil {
		return fmt.Errorf("fail to get cluster chartPath: %v", err)
	}
	ioStreams.Info("Versions:")
	ioStreams.Infof("oam-kubernetes-runtime: %s \n", clusterVersion)
	// TODO(wonderflow): we should print all helm charts installed by vela, including plugins

	return nil
}

func NewInstallCommand(c types.Args, chartContent string, ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &initCmd{ioStreams: ioStreams}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Initialize vela on both client and server",
		Long:  "Install OAM runtime and vela builtin capabilities.",
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			i.client = newClient
			i.namespace = types.DefaultOAMNS
			return i.run(ioStreams, chartContent)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
	}

	flag := cmd.Flags()
	flag.StringVarP(&i.chartPath, "vela-chart-path", "p", "", "path to vela core chart to override default chart")
	flag.StringVarP(&i.chartArgs.imagePullPolicy, "image-pull-policy", "", "Always", "vela core image pull policy, this will align to chart value image.pullPolicy")
	flag.StringVarP(&i.chartArgs.imageRepo, "image-repo", "", "oamdev/vela-core", "vela core image repo, this will align to chart value image.repo")
	flag.StringVarP(&i.chartArgs.imageTag, "image-tag", "", "latest", "vela core image repo, this will align to chart value image.tag")
	flag.BoolVarP(&i.waitReady, "wait", "w", false, "wait until vela-core is ready to serve")

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams, chartSource string) error {
	ioStreams.Info("- Installing Vela Core Chart:")
	exist, err := cmdutil.DoesNamespaceExist(i.client, types.DefaultOAMNS)
	if err != nil {
		return err
	}
	if !exist {
		if err := cmdutil.NewNamespace(i.client, types.DefaultOAMNS); err != nil {
			return err
		}
		ioStreams.Info("created namespace", types.DefaultOAMNS)
	}

	if oam.IsHelmReleaseRunning(types.DefaultOAMReleaseName, types.DefaultOAMRuntimeChartName, i.ioStreams) {
		i.ioStreams.Info("Vela system along with OAM runtime already exist.")
	} else {
		vals, err := i.resolveValues()
		if err != nil {
			i.ioStreams.Errorf("resolve values for vela-core chart err %v, will install with default values", err)
			vals = make(map[string]interface{})
		}
		if err := InstallOamRuntime(i.chartPath, chartSource, vals, ioStreams); err != nil {
			return err
		}
	}

	if err := RefreshDefinitions(context.Background(), i.client, ioStreams); err != nil {
		return err
	}
	ioStreams.Info("- Finished successfully.")

	if i.waitReady {
		_, err := PrintTrackVelaRuntimeStatus(context.Background(), i.client, ioStreams)
		if err != nil {
			return err
		}
	}
	return nil
}

func (i *initCmd) resolveValues() (map[string]interface{}, error) {
	finalValues := map[string]interface{}{}
	valuesConfig := []string{
		//TODO(wonderflow) values here could give more arguments in command line
		fmt.Sprintf("image.repository=%s", i.chartArgs.imageRepo),
		fmt.Sprintf("image.tag=%s", i.chartArgs.imageTag),
		fmt.Sprintf("image.pullPolicy=%s", i.chartArgs.imagePullPolicy),
	}
	for _, val := range valuesConfig {
		// parses Helm strvals line and merges into a map for the final overrides for values.yaml
		if err := strvals.ParseInto(val, finalValues); err != nil {
			return nil, err
		}
	}
	return finalValues, nil
}

func InstallOamRuntime(chartPath, chartSource string, vals map[string]interface{}, ioStreams cmdutil.IOStreams) error {
	var err error
	var chartRequested *chart.Chart
	if chartPath != "" {
		ioStreams.Infof("Use customized chart at: %s", chartPath)
		chartRequested, err = loader.Load(chartPath)
	} else {
		chartRequested, err = cli.LoadChart(chartSource)
		if chartRequested != nil {
			m, l := chartRequested.Metadata, len(chartRequested.Raw)
			ioStreams.Infof("install chart %s, version %s, desc : %s, contains %d file\n", m.Name, m.Version, m.Description, l)
		}
	}
	if err != nil {
		return fmt.Errorf("error loading chart for installation: %s", err)
	}
	installClient, err := oam.NewHelmInstall("", types.DefaultOAMNS, types.DefaultOAMReleaseName)
	if err != nil {
		return fmt.Errorf("error create helm install client: %s", err)
	}
	release, err := installClient.Run(chartRequested, vals)
	if err != nil {
		ioStreams.Errorf("Failed to install the chart with error: %+v\n", err)
		return err
	}
	ioStreams.Infof("Successfully installed the chart, status: %s, last deployed time = %s\n",
		release.Info.Status,
		release.Info.LastDeployed.String())
	return nil
}

func GetOAMReleaseVersion() (string, error) {
	results, err := oam.GetHelmRelease()
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultOAMRuntimeChartName {
			return result.Chart.AppVersion(), nil
		}
	}
	return "", errors.New("oam-kubernetes-runtime not found in your kubernetes cluster, try `vela install` to install")
}

func PrintTrackVelaRuntimeStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams) (bool, error) {
	trackTimeout := 5 * time.Minute
	trackInterval := 2 * time.Second

	ioStreams.Info("\nIt may take 1-2 minutes before KubeVela runtime is ready.")
	start := time.Now()
	spiner := newTrackingSpinner("Waiting KubeVela runtime ready to serve ...")
	spiner.Start()
	defer spiner.Stop()

	for {
		timeConsumed := int(time.Since(start).Seconds())
		applySpinnerNewSuffix(spiner, fmt.Sprintf("Waiting KubeVela runtime ready to serve (timeout %d/%d seconds) ...",
			timeConsumed, int(trackTimeout.Seconds())))

		sts, podName, err := getVelaRuntimeStatus(ctx, c)
		if err != nil {
			return false, err
		}
		if sts == Ready {
			ioStreams.Info(fmt.Sprintf("\n%s %s", emojiSucceed, "KubeVela runtime is ready to serve!"))
			return true, nil
		}
		// status except Ready results in re-check until timeout
		if time.Since(start) > trackTimeout {
			ioStreams.Info(fmt.Sprintf("\n%s %s", emojiFail, "KubeVela runtime starts timeout!"))
			if len(podName) != 0 {
				ioStreams.Info(fmt.Sprintf("\n%s %s%s", emojiLightBulb,
					"Please use this command for more detail: ",
					white.Sprintf("kubectl logs -f %s -n vela-system", podName)))
			}
			return false, nil
		}
		time.Sleep(trackInterval)
	}
}

func getVelaRuntimeStatus(ctx context.Context, c client.Client) (VelaRuntimeStatus, string, error) {
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(types.DefaultOAMNS),
		client.MatchingLabels{
			"app.kubernetes.io/name":     types.DefaultOAMRuntimeChartName,
			"app.kubernetes.io/instance": types.DefaultOAMReleaseName,
		},
	}
	if err := c.List(ctx, podList, opts...); err != nil {
		return Error, "", err
	}
	if len(podList.Items) == 0 {
		return NotFound, "", nil
	}
	runtimePod := podList.Items[0]
	podName := runtimePod.GetName()
	if runtimePod.Status.Phase == corev1.PodRunning {
		// since readiness & liveness probes are set for vela container
		// so check each condition is ready
		for _, c := range runtimePod.Status.Conditions {
			if c.Status != corev1.ConditionTrue {
				return Pending, podName, nil
			}
		}
		return Ready, podName, nil
	}
	return Pending, podName, nil
}
