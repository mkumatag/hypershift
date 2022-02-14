package ibmcloud_powervs

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	"io/ioutil"
	"os"

	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/openshift/hypershift/cmd/log"
	"github.com/spf13/cobra"
)

type CreateInfraOptions struct {
	CloudInstanceID  string
	Region           string
	Zone             string
	InfraID          string
	IsManagedInfra   string
	ManagedInfraJson string
}

type PowerVSNode struct {
	NodeID           string   `json:"nodeId"`
	PrivateNetworkID []string `json:"privateNetworkID"`
}

type ManagedInfra struct {
	NodeCount            int           `json:"nodeCount"`
	PowerVSNodes         []PowerVSNode `json:"powerVSNode"`
	PowerVSPrivateSubnet []string      `json:"powerVSPrivateSubnet"`
	CloudConnectionID    string        `json:"cloudConnectionId"`
	VpcID                string        `json:"vpcId"`
	VpcSubnetID          string        `json:"vpcSubnetId"`
	LoadBalancerID       string        `json:"loadBalancerId"`
}

func NewCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "powervs",
		Short:        "Creates PowerVS infrastructure resources for a cluster",
		SilenceUsage: true,
	}

	opts := CreateInfraOptions{}

	cmd.Flags().StringVar(&opts.CloudInstanceID, "cloud-instance-id", opts.CloudInstanceID, "IBM Cloud InstanceID for PowerVS Environment")
	cmd.Flags().StringVar(&opts.Region, "region", opts.Region, "IBM Cloud Region")
	cmd.Flags().StringVar(&opts.Zone, "zone", opts.Zone, "IBM Cloud Zone")
	cmd.Flags().StringVar(&opts.InfraID, "infra-id", opts.InfraID, "Cluster ID with which to tag PowerVS resources (required)")
	cmd.Flags().StringVar(&opts.IsManagedInfra, "is-managed-infra", opts.IsManagedInfra, "Flag to mention user managed PowerVS resources or not")
	cmd.Flags().StringVar(&opts.ManagedInfraJson, "managed-infra-json", opts.ManagedInfraJson, "If is-managed-infra is yes, JSON Path to information about PowerVS resources")

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
	if o.CloudInstanceID == "" {
		o.CloudInstanceID = os.Getenv("IBMCLOUD_INSTANCE_ID")

		if o.CloudInstanceID == "" {
			return fmt.Errorf("cloud-instance-id is required for PowerVS Infra")
		}
	}

	if o.Region == "" {
		o.Region = os.Getenv("IBMCLOUD_REGION")

		if o.Region == "" {
			return fmt.Errorf("region is required for PowerVS Infra")
		}
	}

	if o.IsManagedInfra == "yes" {
		if len(o.ManagedInfraJson) <= 0 {
			return fmt.Errorf("managed-infra-json should be provided when is-managed-infra set to yes")
		}

		err := validateManagedInfra(o)
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

func createPowerVSSession(createOpt *CreateInfraOptions) *ibmpisession.IBMPISession {

	opt := &ibmpisession.IBMPIOptions{Authenticator: getIAMAuth(),
		Debug:       true,
		Region:      createOpt.Region,
		UserAccount: createOpt.CloudInstanceID,
		Zone:        createOpt.Zone}
	session, _ := ibmpisession.NewIBMPISession(opt)
	return session
}

func createVpcService(createOpt *CreateInfraOptions) *vpcv1.VpcV1 {
	v1, _ := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: getIAMAuth(),
	})
	return v1
}

func validateManagedInfra(createOpt *CreateInfraOptions) error {
	rawJson, err := ioutil.ReadFile(createOpt.ManagedInfraJson)
	if err != nil {
		return fmt.Errorf("failed to read managed infra json: %w", err)
	}

	var managedInfra = &ManagedInfra{}
	if err = json.Unmarshal(rawJson, managedInfra); err != nil {
		return fmt.Errorf("failed to load infra json: %w", err)
	}

	if managedInfra != nil {
		log.Log.Info("ManagedInfra Provided: %+v", managedInfra)
		ibmPiSession := createPowerVSSession(createOpt)
		vpcService := createVpcService(createOpt)

		err = managedInfra.validatePowerVSSubnet(ibmPiSession, createOpt)
		if err != nil {
			return fmt.Errorf("error validating PowerVS Subnet: %w", err)
		}

		err = managedInfra.validatePowerVSInstance(ibmPiSession, createOpt)
		if err != nil {
			return fmt.Errorf("error validating PowerVS Instance: %w", err)
		}

		err = managedInfra.validateVpc(vpcService)
	}
	return nil
}

func (managedInfra *ManagedInfra) validatePowerVSSubnet(session *ibmpisession.IBMPISession, option *CreateInfraOptions) error {
	pvNetworkClient := instance.NewIBMPINetworkClient(context.Background(), session, option.CloudInstanceID)
	for _, subnetId := range managedInfra.PowerVSPrivateSubnet {
		subnet, err := pvNetworkClient.Get(subnetId)
		if err != nil {
			return fmt.Errorf("error validating subnet: %s, error: %w", subnetId, err)
		}

		if *subnet.Type != "vlan" {
			return fmt.Errorf("error validating subnet: %s, provided network is not private", subnetId)
		}

		log.Log.Info("Validated subnet: %s", subnetId)
	}
	return nil
}

func (managedInfra *ManagedInfra) validatePowerVSInstance(session *ibmpisession.IBMPISession, option *CreateInfraOptions) error {
	pvInstanceClient := instance.NewIBMPIInstanceClient(context.Background(), session, option.CloudInstanceID)
	for _, node := range managedInfra.PowerVSNodes {
		pvInstance, err := pvInstanceClient.Get(node.NodeID)

		if err != nil {
			return fmt.Errorf("error validating node: %s, error: %w", node.NodeID, err)
		}
		networkCheckM := map[string]bool{}
		for _, nwId := range node.PrivateNetworkID {
			networkCheckM[nwId] = false
		}
		for _, nw := range pvInstance.Addresses {
			_, exist := networkCheckM[nw.NetworkID]
			if exist {
				networkCheckM[nw.NetworkID] = true
			}
		}
		for nwId, exist := range networkCheckM {
			if !exist {
				return fmt.Errorf("error validating node: %s, network: %s is Invalid", node.NodeID, nwId)
			}
		}

		log.Log.Info("Validated Node: %s", node.NodeID)
	}
	return nil
}

func (managedInfra *ManagedInfra) validateVpc(v1 *vpcv1.VpcV1) error {
	getVpcOpt := vpcv1.GetVPCOptions{ID: &managedInfra.VpcID}
	vpcResult, _, err := v1.GetVPC(&getVpcOpt)

	if err != nil {
		return fmt.Errorf("error validating VPC: %s, error: %w", managedInfra.VpcID, err)
	}
	if vpcResult != nil || *vpcResult.ID != managedInfra.VpcID {
		return fmt.Errorf("error validating VPC: %s, received invalid VPC", managedInfra.VpcID)
	}

	getSubnetOpt := vpcv1.GetSubnetOptions{ID: &managedInfra.VpcSubnetID}
	vpcSubnetResult, _, err := v1.GetSubnet(&getSubnetOpt)

	if err != nil {
		return fmt.Errorf("error validating VPC Subnet: %s, error: %w", managedInfra.VpcSubnetID, err)
	}
	if vpcSubnetResult != nil || *vpcSubnetResult.ID != managedInfra.VpcSubnetID {
		return fmt.Errorf("error validating VPC: %s, received invalid VPC Subnet", managedInfra.VpcSubnetID)
	}

	getLbOpt := vpcv1.GetLoadBalancerOptions{ID: &managedInfra.LoadBalancerID}
	vpcLbResult, _, err := v1.GetLoadBalancer(&getLbOpt)

	if err != nil {
		return fmt.Errorf("error validating VPC LoadBalancer: %s, error: %w", managedInfra.LoadBalancerID, err)
	}
	if vpcLbResult != nil || *vpcLbResult.ID != managedInfra.VpcSubnetID {
		return fmt.Errorf("error validating VPC: %s, received invalid VPC LoadBalancer", managedInfra.LoadBalancerID)
	}
	return nil
}
