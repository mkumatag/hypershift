package common

import (
	"github.com/openshift/hypershift/cmd/cluster/core"
	"github.com/spf13/cobra"
)

func AddCommonFlags(cmd *cobra.Command, opts *core.CreateOptions) *core.IBMCloudPlatformOptions {
	opt := &core.IBMCloudPlatformOptions{}
	cmd.Flags().StringVar(&opt.VPC, "vpc", "", "VPC Name")
	cmd.Flags().StringVar(&opt.APIKey, "api-key", "", "API Key")
	cmd.Flags().StringVar(&opt.BaseDomain, "base-domain", "", "The ingress base domain for the cluster")

	cmd.MarkFlagRequired("api-key")
	return opt
}
