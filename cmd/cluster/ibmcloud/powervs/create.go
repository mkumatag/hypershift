package powervs

import (
	"context"
	"fmt"
	"github.com/openshift/hypershift/cmd/cluster/ibmcloud/common"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"os"
	"os/signal"
	"syscall"

	apifixtures "github.com/openshift/hypershift/api/fixtures"
	"github.com/openshift/hypershift/cmd/cluster/core"
	"github.com/spf13/cobra"
)

func NewCreateCommand(opts *core.CreateOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "powervs",
		Short:        "Creates basic functional HostedCluster resources on IBMCloudPowerVS PowerVS",
		SilenceUsage: true,
	}

	commonFlags := common.AddCommonFlags(cmd, opts)

	opts.IBMCloudPowerVSPlatform = core.IBMCloudPowerVSPlatformOptions{
		IBMCloudPlatformOptions: commonFlags,
	}

	cmd.Run = func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT)
		go func() {
			<-sigs
			cancel()
		}()

		if err := CreateCluster(ctx, opts); err != nil {
			log.Error(err, "Failed to create cluster")
			os.Exit(1)
		}
	}

	return cmd
}

func CreateCluster(ctx context.Context, opts *core.CreateOptions) error {
	return core.CreateCluster(ctx, opts, applyPlatformSpecificsValues)
}

func applyPlatformSpecificsValues(ctx context.Context, exampleOptions *apifixtures.ExampleOptions, opts *core.CreateOptions) (err error) {
	infraID := opts.InfraID
	if len(infraID) == 0 {
		infraID = fmt.Sprintf("%s-%s", opts.Name, utilrand.String(5))
	}

	exampleOptions.BaseDomain = opts.IBMCloudPowerVSPlatform.BaseDomain

	exampleOptions.IBMCloudPowerVS = &apifixtures.ExampleIBMCloudPowerVSOptions{}
	exampleOptions.InfraID = infraID
	exampleOptions.IBMCloudPowerVS.ApiKey = opts.IBMCloudPowerVSPlatform.APIKey
	return nil
}
