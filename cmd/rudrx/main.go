package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/spf13/cobra"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloud-native-application/rudrx/pkg/cmd"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/utils/logs"
)

// noUsageError suppresses usage printing when it occurs
// (since cobra doesn't provide a good way to avoid printing
// out usage in only certain situations).
type noUsageError struct{ error }

var (
	scheme = k8sruntime.NewScheme()

	// RudrxVersion is the version of cli.
	RudrxVersion = "UNKNOWN"

	// GitRevision is the commit of repo
	GitRevision = "UNKNOWN"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	rand.Seed(time.Now().UnixNano())

	command := newCommand()

	logs.InitLogs()
	defer logs.FlushLogs()

	command.Execute()
}

func newCommand() *cobra.Command {
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	cmds := &cobra.Command{
		Use:          "rudrx",
		Short:        "✈️  A Micro App Plafrom for Kubernetes.",
		Long:         "✈️  A Micro App Plafrom for Kubernetes.",
		Run:          runHelp,
		SilenceUsage: true,
	}

	flags := cmds.PersistentFlags()
	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	kubeConfigFlags.AddFlags(flags)
	f := cmdutil.NewFactory(kubeConfigFlags)
	restConf, err := f.ToRESTConfig()
	if err != nil {
		fmt.Println("get kubeconfig err", err)
		os.Exit(1)
	}
	client, err := client.New(restConf, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Println("create client from kubeconfig err", err)
		os.Exit(1)
	}
	if err := system.InitApplicationDir(); err != nil {
		fmt.Println("InitApplicationDir err", err)
		os.Exit(1)
	}
	if err := system.InitDefinitionDir(); err != nil {
		fmt.Println("InitDefinitionDir err", err)
		os.Exit(1)
	}

	cmds.AddCommand(
		cmd.NewTraitsCommand(f, client, ioStream, []string{}),
		cmd.NewWorkloadsCommand(f, client, ioStream, os.Args[1:]),
		cmd.NewInitCommand(f, client, ioStream),
		cmd.NewDeleteCommand(f, client, ioStream, os.Args[1:]),
		cmd.NewAppsCommand(f, client, ioStream),
		cmd.NewEnvInitCommand(f, ioStream),
		cmd.NewEnvSwitchCommand(f, ioStream),
		cmd.NewEnvDeleteCommand(f, ioStream),
		cmd.NewEnvCommand(f, ioStream),
		NewVersionCommand(),
		cmd.NewAppStatusCommand(client, ioStream),
		cmd.NewCompletionCommand(),
	)
	if err = cmd.AddWorkloadPlugins(cmds, client, ioStream); err != nil {
		fmt.Println("Add plugins from workloadDefinition err", err)
		os.Exit(1)
	}
	if err = cmd.AddTraitPlugins(cmds, client, ioStream); err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}
	if err = cmd.DetachTraitPlugins(cmds, client, ioStream); err != nil {
		fmt.Println("Add plugins from traitDefinition err", err)
		os.Exit(1)
	}
	return cmds
}

func runHelp(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints out build version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(`Version: %v
GitRevision: %v
GolangVersion: %v
`,
				RudrxVersion,
				GitRevision,
				runtime.Version())
		},
	}
}
