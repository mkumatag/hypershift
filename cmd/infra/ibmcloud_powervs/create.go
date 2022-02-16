package ibmcloud_powervs

import (
	"context"
	"fmt"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/platform-services-go-sdk/resourcemanagerv2"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	powerUtils "github.com/ppc64le-cloud/powervs-utils"
	"github.com/spf13/cobra"
	"os"
	"sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/resourcecontroller"

	"github.com/openshift/hypershift/cmd/log"
	servicesUtils "sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/utils"
)

type CreateInfraOptions struct {
	ResourceGroup          string
	InfraID                string
	NodePoolReplicas       int
	PowerVSRegion          string
	PowerVSZone            string
	PowerVSCloudInstanceID string
	PowerVSInstanceID      []string
	PowerVSSubnetID        string
	VpcRegion              string
	VpcID                  string
	VpcSubnetID            string
	VpcLoadBalancerID      string
	CloudConnectionID      string
}

type CloudInstanceOption struct {
	Name           string `json:"name"`
	ResourcePlanId string `json:"resource_plan_id"`
	ResourceGroup  string `json:"resource_group"`
	Target         string `json:"target"`
	AllowCleanup   bool   `json:"allow_cleanup"`
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

	cmd.Flags().StringVar(&opts.ResourceGroup, "resource-group", opts.ResourceGroup, "IBM Cloud Resource Group")
	cmd.Flags().StringVar(&opts.InfraID, "infra-id", opts.InfraID, "Cluster ID with which to tag IBM Cloud resources")
	cmd.Flags().IntVar(&opts.NodePoolReplicas, "node-pool-replicas", 1, "If >-1, create a default NodePool with this many replicas")
	cmd.Flags().StringVar(&opts.PowerVSRegion, "pv-region", opts.PowerVSRegion, "IBM Cloud PowerVS Region")
	cmd.Flags().StringVar(&opts.PowerVSZone, "pv-zone", opts.PowerVSZone, "PowerVS Region's Zone")
	cmd.Flags().StringVar(&opts.PowerVSCloudInstanceID, "pv-cloud-instance-id", opts.PowerVSCloudInstanceID, "IBM Cloud InstanceID of PowerVS Resource Group")
	cmd.Flags().StringSlice("pv-instance-id", opts.PowerVSInstanceID, "List of PowerVS Virtual Server Instance worker node's ID in a comma separated list")
	cmd.Flags().StringVar(&opts.PowerVSSubnetID, "pv-subnet-id", opts.PowerVSSubnetID, "PowerVS Private Subnet ID attached to the PowerVS Instance")
	cmd.Flags().StringVar(&opts.VpcRegion, "vpc-region", opts.VpcRegion, "IBM Cloud VPC Region for VPC resources")
	cmd.Flags().StringVar(&opts.VpcID, "vpc-id", opts.VpcID, "IBM Cloud VPC ID")
	cmd.Flags().StringVar(&opts.VpcSubnetID, "vpc-subnet-id", opts.VpcSubnetID, "VPC Subnet ID")
	cmd.Flags().StringVar(&opts.VpcLoadBalancerID, "vpc-load-balancer", opts.VpcLoadBalancerID, "VPC Load Balancer")
	cmd.Flags().StringVar(&opts.CloudConnectionID, "cloud-connection-id", opts.CloudConnectionID, "IBM Cloud Cloud Connection ID")

	cmd.MarkFlagRequired("resource-group")
	cmd.MarkFlagRequired("infra-id")
	cmd.MarkFlagRequired("pv-region")
	cmd.MarkFlagRequired("pv-zone")
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

func (option *CreateInfraOptions) Run(ctx context.Context) error {
	if option.PowerVSCloudInstanceID == "" {
		var err error
		option.PowerVSCloudInstanceID, err = createCloudInstance(option)
		if err != nil {
			return fmt.Errorf("error creating cloud instance: %w", err)
		}
	}

	session, err := createPowerVSSession(option)
	if err != nil {
		return fmt.Errorf("error creating PowerVS session: %w", err)
	}
	vpcv1, err := createVpcService(option)
	if err != nil {
		return fmt.Errorf("error creating VPC service: %w", err)
	}

	err = setupInfra(option, session, vpcv1)
	if err != nil {
		return err
	}

	return nil
}

func setupInfra(option *CreateInfraOptions, session *ibmpisession.IBMPISession, vpc1 *vpcv1.VpcV1) error {
	/*err := setupPowerVsInstance(option, session)
	if err != nil {
		return fmt.Errorf("error setup powervs instance: %w", err)
	}*/
	return nil
}

func getResourceGroupID(options *CreateInfraOptions) (string, error) {
	var resourceGroupID string

	rmv2, err := resourcemanagerv2.NewResourceManagerV2(&resourcemanagerv2.ResourceManagerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return resourceGroupID, err
	}

	fmt.Println("created rmv2")
	rmv2ListResourceGroupOpt := resourcemanagerv2.ListResourceGroupsOptions{Name: &options.ResourceGroup}
	resourceGroupListResult, _, err := rmv2.ListResourceGroups(&rmv2ListResourceGroupOpt)
	if err != nil {
		return resourceGroupID, err
	}

	fmt.Println("listed rgs")
	for _, rg := range resourceGroupListResult.Resources {
		if *rg.Name == options.ResourceGroup {
			resourceGroupID = *rg.ID
		}
	}

	fmt.Println("returning", resourceGroupID)
	return resourceGroupID, nil
}
func createCloudInstance(options *CreateInfraOptions) (string, error) {

	var cloudInstanceID string
	resourceGroupID, err := getResourceGroupID(options)
	if err != nil {
		return cloudInstanceID, err
	}

	rcv2, err := resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return cloudInstanceID, err
	}
	fmt.Println("created rcv2")

	cloudInstanceName := fmt.Sprintf("%s-hypershift-nodepool", options.InfraID)
	defaultServicePlanID := "f165dd34-3a40-423b-9d95-e90a23f724dd"
	target := options.PowerVSZone
	resourceInstanceOpt := resourcecontrollerv2.CreateResourceInstanceOptions{Name: &cloudInstanceName,
		ResourceGroup:  &resourceGroupID,
		ResourcePlanID: &defaultServicePlanID,
		Target:         &target}

	fmt.Printf("resource instance opt: %+v\n", resourceInstanceOpt)

	resourceInstance, _, err := rcv2.CreateResourceInstance(&resourceInstanceOpt)

	fmt.Printf("resource instance created: %+v\n", resourceInstance)
	return *resourceInstance.ID, err
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
			ID: core.StringPtr(createOpt.PowerVSCloudInstanceID),
		})
	if err != nil {
		return nil, fmt.Errorf("error collecting resource for cloud instance %s, error: %w", createOpt.PowerVSCloudInstanceID, err)
	}

	region, err := powerUtils.GetRegion(*res.RegionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get region for cloud instance %s, error: %w", createOpt.PowerVSCloudInstanceID, err)
	}

	opt := &ibmpisession.IBMPIOptions{Authenticator: auth,
		Debug:       true,
		Region:      region,
		UserAccount: account,
		Zone:        *res.RegionID}

	session, err := ibmpisession.NewIBMPISession(opt)
	fmt.Println("IBM PI Session created")
	return session, err
}

func createVpcService(createOpt *CreateInfraOptions) (*vpcv1.VpcV1, error) {
	v1, err := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: getIAMAuth(),
		URL:           fmt.Sprintf("https://%s.iaas.cloud.ibm.com/v1", createOpt.VpcRegion),
	})
	fmt.Println("VPCV1 created")
	return v1, err
}

func setupPowerVsInstance(option *CreateInfraOptions, session *ibmpisession.IBMPISession) error {
	if len(option.PowerVSInstanceID) > 0 {

	}
	return nil
}
