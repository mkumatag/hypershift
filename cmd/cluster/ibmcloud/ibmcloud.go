package ibmcloud

import (
	"github.com/openshift/hypershift/cmd/cluster/core"
	"github.com/openshift/hypershift/cmd/cluster/ibmcloud/powervs"
	"github.com/openshift/hypershift/cmd/cluster/ibmcloud/vpc"
	"github.com/spf13/cobra"
)

func NewCreateCommand(opts *core.CreateOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "ibmcloud",
		Short:        "Creates basic functional HostedCluster resources on IBMCloudPowerVS",
		SilenceUsage: true,
	}

	cmd.AddCommand(vpc.NewCreateCommand(opts))
	cmd.AddCommand(powervs.NewCreateCommand(opts))
	return cmd
}
