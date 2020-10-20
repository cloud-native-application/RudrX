package commands

import (
	"errors"
	"fmt"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/commands/util"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/plugins"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Staging      = "staging"
	App          = "app"
	WorkloadType = "type"
	TraitDetach  = "detach"
)

type runOptions oam.RunOptions

func newRunOptions(ioStreams util.IOStreams) *runOptions {
	return &runOptions{IOStreams: ioStreams}
}

func AddCompCommands(c types.Args, ioStreams util.IOStreams) *cobra.Command {
	compCommands := &cobra.Command{
		Use:                   "comp",
		DisableFlagsInUseLine: true,
		Short:                 "Manage Components",
		Long:                  "Manage Components",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	compCommands.PersistentFlags().StringP(App, "a", "", "specify application name for component")

	compCommands.AddCommand(
		NewCompListCommand(c, ioStreams),
		NewCompDeployCommands(c, ioStreams),
		NewCompShowCommand(ioStreams),
		NewCompStatusCommand(c, ioStreams),
		NewCompDeleteCommand(c, ioStreams),
	)
	return compCommands
}

func NewCompDeployCommands(c types.Args, ioStreams util.IOStreams) *cobra.Command {
	runCmd := &cobra.Command{
		Use:                   "deploy [args]",
		DisableFlagsInUseLine: true,
		// Dynamic flag parse in compeletion
		DisableFlagParsing: true,
		Short:              "Init and Run workloads",
		Long:               "Init and Run workloads",
		Example:            "vela comp deploy -t <workload-type>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || args[0] == "-h" {
				err := cmd.Help()
				if err != nil {
					return err
				}
				return nil
			}
			o := newRunOptions(ioStreams)
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			o.KubeClient = newClient
			o.Env, err = GetEnv(cmd)
			if err != nil {
				return err
			}
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			return o.Run(cmd, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	runCmd.SetOut(ioStreams.Out)

	runCmd.Flags().BoolP(Staging, "s", false, "only save changes locally without real update application")
	runCmd.Flags().StringP(WorkloadType, "t", "", "specify workload type for application")

	return runCmd
}

func GetWorkloadNameFromArgs(args []string) (string, error) {
	argsLength := len(args)
	if argsLength < 1 {
		return "", errors.New("must specify name for component")
	}
	return args[0], nil

}

func (o *runOptions) Complete(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	flags.AddFlagSet(cmd.PersistentFlags())
	flags.ParseErrorsWhitelist.UnknownFlags = true

	// First parse, figure out which workloadType it is.
	if err := flags.Parse(args); err != nil {
		return err
	}

	workloadName, err := GetWorkloadNameFromArgs(flags.Args())
	if err != nil {
		return err
	}

	appName, err := flags.GetString(App)
	if err != nil {
		return err
	}
	workloadType, err := flags.GetString(WorkloadType)
	if err != nil {
		return err
	}
	if workloadType == "" {
		workloads, err := plugins.LoadInstalledCapabilityWithType(types.TypeWorkload)
		if err != nil {
			return err
		}
		var workloadList []string
		for _, w := range workloads {
			workloadList = append(workloadList, w.Name)
		}
		errMsg := "can not find workload, check workloads by `vela workloads` and choose a suitable one."
		if workloadList != nil {
			errMsg = fmt.Sprintf("must specify type of workload, please use `-t` and choose from %v.", workloadList)
		}
		return errors.New(errMsg)
	}
	envName := o.Env.Name

	// Dynamic load flags
	template, err := plugins.LoadCapabilityByName(workloadType)
	if err != nil {
		return err
	}
	for _, v := range template.Parameters {
		types.SetFlagBy(flags, v)
	}
	// Second parse, parse parameters of this workload.
	if err = flags.Parse(args); err != nil {
		return err
	}
	app, err := oam.BaseComplete(envName, workloadName, appName, flags, workloadType)
	if err != nil {
		return err
	}

	o.App = app
	return err
}

func (o *runOptions) Run(cmd *cobra.Command, io cmdutil.IOStreams) error {
	staging, err := cmd.Flags().GetBool(Staging)
	if err != nil {
		return err
	}
	msg, err := oam.BaseRun(staging, o.App, o.KubeClient, o.Env, io)
	if err != nil {
		return err
	}
	o.Info(msg)
	return nil
}
