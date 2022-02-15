package ibmcloud_powervs

import (
	"context"
	"fmt"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	"os"

	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/openshift/hypershift/cmd/log"
	powerUtils "github.com/ppc64le-cloud/powervs-utils"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/resourcecontroller"
	servicesUtils "sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/utils"
)

type CreateInfraOptions struct {
	CloudInstanceID  string
	VPCRegion        string
	InfraID          string
	IsManagedInfra   string
	ManagedInfraJson string
}

type PowerVSNode struct {
	NodeID           string   `json:"nodeId"`
	PrivateNetworkID []string `json:"privateNetworkID"`
}

type VPC struct {
	ID             string `json:"id"`
	SubnetID       string `json:"subnetId"`
	LoadBalancerID string `json:"loadBalancerId"`
}

type ManagedInfra struct {
	NodeCount            int           `json:"nodeCount"`
	PowerVSNode          []PowerVSNode `json:"powerVSNode"`
	PowerVSPrivateSubnet []string      `json:"powerVSPrivateSubnet"`
	CloudConnectionID    string        `json:"cloudConnectionId"`
	Vpc                  []VPC         `json:"vpc"`
}

func NewCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "powervs",
		Short:        "Creates PowerVS infrastructure resources for a cluster",
		SilenceUsage: true,
	}

	opts := CreateInfraOptions{}

	cmd.Flags().StringVar(&opts.CloudInstanceID, "pv-cloud-instance-id", opts.CloudInstanceID, "IBM Cloud InstanceID for PowerVS Environment")
	cmd.Flags().StringVar(&opts.VPCRegion, "vpc-region", opts.VPCRegion, "IBM Cloud VPC Region for VPC resources")
	cmd.Flags().StringVar(&opts.InfraID, "infra-id", opts.InfraID, "Cluster ID with which to tag PowerVS resources (required)")
	cmd.Flags().StringVar(&opts.IsManagedInfra, "is-managed-infra", opts.IsManagedInfra, "Flag to mention user managed PowerVS resources or not")
	cmd.Flags().StringVar(&opts.ManagedInfraJson, "managed-infra-json", opts.ManagedInfraJson, "If is-managed-infra is yes, JSON Path to information about PowerVS resources")

	cmd.MarkFlagRequired("pv-cloud-instance-id")
	cmd.MarkFlagRequired("vpc-region")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := opts.Run(cmd.Context()); err != nil {
			log.Log.Error(err, "Failed to create infrastructure")
			return err
		}
		log.Log.Info("Successfully created infrastructure")
		return nil
	}

	return cmd
}

func (o *CreateInfraOptions) Run(ctx context.Context) error {

	if o.IsManagedInfra == "yes" {
		if len(o.ManagedInfraJson) <= 0 {
			return fmt.Errorf("managed-infra-json should be provided when is-managed-infra set to yes")
		}

		session, err := createPowerVSSession(o)
		if err != nil {
			return fmt.Errorf("error creating PowerVS session: %w", err)
		}
		vpcv1, err := createVpcService(o)
		if err != nil {
			return fmt.Errorf("error creating VPC service: %w", err)
		}

		err = validateManagedInfra(o, session, vpcv1)
		if err != nil {
			return err
		}
	}

	return nil
}

func getIAMAuth() *core.IamAuthenticator {
	return &core.IamAuthenticator{
		ApiKey: os.Getenv("IBMCLOUD_API_KEY"),
	}
}

func createPowerVSSession(createOpt *CreateInfraOptions) (*ibmpisession.IBMPISession, error) {
	auth := getIAMAuth()
	account, err := servicesUtils.GetAccount(auth)

	if err != nil {
		return nil, fmt.Errorf("error retrieving account: %w", err)
	}

	rc, err := resourcecontroller.NewService(resourcecontroller.ServiceOptions{})
	if err != nil {
		return nil, err
	}

	res, _, err := rc.GetResourceInstance(
		&resourcecontrollerv2.GetResourceInstanceOptions{
			ID: core.StringPtr(createOpt.CloudInstanceID),
		})
	if err != nil {
		return nil, fmt.Errorf("error collecting resource for cloud instance %s, error: %w", createOpt.CloudInstanceID, err)
	}

	region, err := powerUtils.GetRegion(*res.RegionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get region for cloud instance %s, error: %w", createOpt.CloudInstanceID, err)
	}

	opt := &ibmpisession.IBMPIOptions{Authenticator: auth,
		Debug:       true,
		Region:      region,
		UserAccount: account,
		Zone:        *res.RegionID}
	log.Log.Info("Printing IBM PI", "options", opt)
	session, err := ibmpisession.NewIBMPISession(opt)
	return session, err
}

func createVpcService(createOpt *CreateInfraOptions) (*vpcv1.VpcV1, error) {
	v1, err := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: getIAMAuth(),
		URL:           fmt.Sprintf("https://%s.iaas.cloud.ibm.com/v1", createOpt.VPCRegion),
	})
	return v1, err
}
