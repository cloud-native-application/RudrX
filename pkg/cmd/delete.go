package cmd

import (
	"errors"
	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/cloud-native-application/rudrx/pkg/oam"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewDeleteCommand Delete App
func NewDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete <APPLICATION_NAME>",
		DisableFlagsInUseLine: true,
		Short:                 "Delete Applications",
		Long:                  "Delete Applications",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela app delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o := &oam.DeleteOptions{}
		o.Client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.AppName = args[0]

		ioStreams.Infof("Deleting Application \"%s\"\n", o.AppName)
		err, _ = o.DeleteApp()
		if err != nil {
			if apierrors.IsNotFound(err) {
				ioStreams.Info("Already deleted")
				return nil
			}
			return err
		}
		ioStreams.Info("DELETE SUCCEED")
		return nil
	}
	return cmd
}

// NewCompDeleteCommand delete component
func NewCompDeleteCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete <ComponentName>",
		DisableFlagsInUseLine: true,
		Short:                 "Delete Component From Application",
		Long:                  "Delete Component From Application",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela comp delete frontend -a frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		appName, err := cmd.Flags().GetString(App)
		if err != nil {
			return err
		}
		if appName == "" {
			return errors.New("must specify name of application, please add flag -a")
		}
		newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
		if err != nil {
			return err
		}
		o := &oam.DeleteOptions{}
		o.Client = newClient
		o.Env, err = GetEnv(cmd)
		if err != nil {
			return err
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.CompName = args[0]
		o.AppName = appName

		ioStreams.Infof("Deleting Component '%s' from Application '%s'\n", o.CompName, o.AppName)
		err, message := o.DeleteComponent()
		if err != nil {
			return err
		}
		ioStreams.Info(message)
		return nil
	}
	return cmd
}
