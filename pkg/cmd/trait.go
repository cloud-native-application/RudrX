package cmd

import (
	"context"
	"errors"

	"github.com/cloud-native-application/rudrx/pkg/oam"

	"github.com/cloud-native-application/rudrx/pkg/application"

	"github.com/cloud-native-application/rudrx/pkg/plugins"

	"github.com/cloud-native-application/rudrx/api/types"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type commandOptions struct {
	Template types.Capability
	Client   client.Client
	Detach   bool
	Env      *types.EnvMeta

	workloadName string
	appName      string
	staging      bool
	app          *application.Application
	cmdutil.IOStreams
}

func NewCommandOptions(ioStreams cmdutil.IOStreams) *commandOptions {
	return &commandOptions{IOStreams: ioStreams}
}

func AddTraitCommands(parentCmd *cobra.Command, c types.Args, ioStreams cmdutil.IOStreams) error {
	templates, err := plugins.LoadInstalledCapabilityWithType(types.TypeTrait)
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, tmp := range templates {
		tmp := tmp

		var name = tmp.Name
		pluginCmd := &cobra.Command{
			Use:                   name + " <appname> [args]",
			DisableFlagsInUseLine: true,
			Short:                 "Attach " + name + " trait to an app",
			Long:                  "Attach " + name + " trait to an app",
			Example:               "vela " + name + " frontend",
			RunE: func(cmd *cobra.Command, args []string) error {
				o := NewCommandOptions(ioStreams)
				o.Template = tmp
				newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
				if err != nil {
					return err
				}
				o.Client = newClient
				o.Env, err = GetEnv(cmd)
				if err != nil {
					return err
				}
				detach, _ := cmd.Flags().GetBool(TraitDetach)
				if detach {
					if err := o.DetachTrait(cmd, args); err != nil {
						return err
					}
					o.Detach = true
				} else {
					if err := o.AddOrUpdateTrait(cmd, args); err != nil {
						return err
					}
				}
				return o.Run(cmd, ctx)
			},
			Annotations: map[string]string{
				types.TagCommandType: types.TypeTraits,
			},
		}
		pluginCmd.SetOut(ioStreams.Out)
		for _, v := range tmp.Parameters {
			types.SetFlagBy(pluginCmd.Flags(), v)
		}
		pluginCmd.Flags().StringP(App, "a", "", "create or add into an existing application group")
		pluginCmd.Flags().BoolP(Staging, "s", false, "only save changes locally without real update application")
		pluginCmd.Flags().BoolP(TraitDetach, "", false, "detach trait from component")

		parentCmd.AddCommand(pluginCmd)
	}
	return nil
}

func (o *commandOptions) Prepare(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("please specify the name of the app")
	}
	o.workloadName = args[0]
	if app := cmd.Flag(App).Value.String(); app != "" {
		o.appName = app
	} else {
		o.appName = o.workloadName
	}
	return nil
}

func (o *commandOptions) AddOrUpdateTrait(cmd *cobra.Command, args []string) error {
	if err := o.Prepare(cmd, args); err != nil {
		return err
	}
	_, err := oam.AddOrUpdateTrait(o.Env.Name, o.appName, o.workloadName, cmd.Flags(), o.Template)
	return err
}

func (o *commandOptions) DetachTrait(cmd *cobra.Command, args []string) error {
	if err := o.Prepare(cmd, args); err != nil {
		return err
	}
	app, err := application.Load(o.Env.Name, o.appName)
	if err != nil {
		return err
	}
	var traitType = o.Template.Name
	if err = app.RemoveTrait(o.workloadName, traitType); err != nil {
		return err
	}
	o.app = app
	return o.app.Save(o.Env.Name)
}

func (o *commandOptions) Run(cmd *cobra.Command, ctx context.Context) error {
	if o.Detach {
		o.Infof("Detaching %s from app %s\n", o.Template.Name, o.workloadName)
	} else {
		o.Infof("Adding %s for app %s \n", o.Template.Name, o.workloadName)
	}
	staging, err := cmd.Flags().GetBool(Staging)
	if err != nil {
		return err
	}
	if staging {
		o.Info("Staging saved")
		return nil
	}
	err = o.app.Run(ctx, o.Client, o.Env)
	if err != nil {
		return err
	}
	o.Info("Succeeded!")
	return nil
}
