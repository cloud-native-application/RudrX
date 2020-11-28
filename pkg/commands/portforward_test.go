package commands

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubectl/pkg/cmd/portforward"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

func TestPortForwardCommand(t *testing.T) {
	fakePod := corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:            "fakePod",
			Namespace:       "default",
			ResourceVersion: "10",
			Labels: map[string]string{
				oam.LabelAppComponent: "fakeComp",
			}},
	}
	tf := cmdtesting.NewTestFactory()
	defer tf.Cleanup()

	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	fakeC := types.Args{
		Config: tf.ClientConfigVal,
	}
	cmd := NewPortForwardCommand(fakeC, io)
	cmd.PersistentFlags().StringP("env", "e", "", "")
	fakeClientSet := k8sfake.NewSimpleClientset(&corev1.PodList{
		Items: []corev1.Pod{fakePod},
	})

	o := &VelaPortForwardOptions{
		ioStreams:            io,
		kcPortForwardOptions: &portforward.PortForwardOptions{},
		f:                    tf,
		ClientSet:            fakeClientSet,
		VelaC:                fakeC,
	}
	err := o.Init(context.Background(), cmd, []string{"fakeApp", "8081:8080"})
	assert.NoError(t, err)
}
