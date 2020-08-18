package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cloud-native-application/rudrx/pkg/oam"

	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/cloud-native-application/rudrx/api/types"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/stretchr/testify/assert"
)

func TestENV(t *testing.T) {
	ctx := context.Background()

	assert.NoError(t, os.Setenv(system.VelaHomeEnv, ".test_vela"))
	home, err := system.GetVelaHomeDir()
	assert.NoError(t, err)
	assert.Equal(t, true, strings.HasSuffix(home, ".test_vela"))
	defer os.RemoveAll(home)
	// Create Default Env
	err = system.InitDefaultEnv()
	assert.NoError(t, err)

	// check and compare create default env success
	curEnvName, err := oam.GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "default", curEnvName)
	gotEnv, err := GetEnv(nil)
	assert.NoError(t, err)
	assert.Equal(t, &types.EnvMeta{
		Namespace: "default",
		Name:      "default",
	}, gotEnv)

	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	exp := &types.EnvMeta{
		Namespace: "test1",
		Name:      "env1",
	}
	client := test.NewMockClient()
	// Create env1
	err = CreateOrUpdateEnv(ctx, client, exp, []string{"env1"}, ioStream)
	assert.NoError(t, err)

	// check and compare create env success
	curEnvName, err = oam.GetCurrentEnvName()
	assert.NoError(t, err)
	assert.Equal(t, "env1", curEnvName)
	gotEnv, err = GetEnv(nil)
	assert.NoError(t, err)
	assert.Equal(t, exp, gotEnv)

	// List all env
	var b bytes.Buffer
	ioStream.Out = &b
	err = ListEnvs(ctx, []string{}, ioStream)
	assert.NoError(t, err)
	assert.Equal(t, "NAME   \tCURRENT\tNAMESPACE\ndefault\t       \tdefault  \nenv1   \t*      \ttest1    \n", b.String())
	b.Reset()
	err = ListEnvs(ctx, []string{"env1"}, ioStream)
	assert.NoError(t, err)
	assert.Equal(t, "NAME\tCURRENT\tNAMESPACE\nenv1\t       \ttest1    \n", b.String())
	ioStream.Out = os.Stdout

	// can not delete current env
	err = DeleteEnv(ctx, []string{"env1"}, ioStream)
	assert.Error(t, err)

	// switch to default env
	err = SwitchEnv(ctx, []string{"default"}, ioStream)
	assert.NoError(t, err)

	// check switch success
	gotEnv, err = GetEnv(nil)
	assert.NoError(t, err)
	assert.Equal(t, &types.EnvMeta{
		Namespace: "default",
		Name:      "default",
	}, gotEnv)

	// delete env
	err = DeleteEnv(ctx, []string{"env1"}, ioStream)
	assert.NoError(t, err)

	// can not switch to non-exist env
	err = SwitchEnv(ctx, []string{"env1"}, ioStream)
	assert.Error(t, err)

	// switch success
	err = SwitchEnv(ctx, []string{"default"}, ioStream)
	assert.NoError(t, err)
}
