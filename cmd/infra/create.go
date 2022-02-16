package infra

import (
	"github.com/openshift/hypershift/cmd/infra/ibmcloud_powervs"
	"github.com/spf13/cobra"

	"github.com/openshift/hypershift/cmd/infra/aws"
)

func NewCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "infra",
		Short:        "Commands for creating HyperShift infra resources",
		SilenceUsage: true,
	}

	cmd.AddCommand(aws.NewCreateCommand())
	cmd.AddCommand(ibmcloud_powervs.NewCreateCommand())

	return cmd
}
