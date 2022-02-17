package ibmcloud_powervs

import (
	"context"
	"fmt"
	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/globalcatalogv1"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/platform-services-go-sdk/resourcemanagerv2"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	cidrutil "github.com/apparentlymart/go-cidr/cidr"
	"github.com/openshift/hypershift/cmd/log"
	powerUtils "github.com/ppc64le-cloud/powervs-utils"
	"github.com/spf13/cobra"
	"net"
	"os"
	servicesUtils "sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/utils"
	"strconv"
	"strings"
)

type CreateInfraOptions struct {
	ServiceName            string
	ServicePlanName        string
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

const basePowerVsPrivateSubnetCIDR = "10.0.0.0/24"

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

	opts := CreateInfraOptions{ServiceName: "power-iaas", ServicePlanName: "power-virtual-server-group"}

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

func (option *CreateInfraOptions) Run(ctx context.Context) (err error) {
	if option.PowerVSCloudInstanceID == "" {
		option.PowerVSCloudInstanceID, err = createCloudInstance(option)
		if err != nil {
			return fmt.Errorf("error creating cloud instance: %w", err)
		}
		log.Log.Info("Cloud Instance Created", "cloudInstanceID", option.PowerVSCloudInstanceID)
	} else {
		err = validateCloudInstance(option.PowerVSCloudInstanceID)
		if err != nil {
			return fmt.Errorf("error validating cloud instance id %s, %w", option.PowerVSCloudInstanceID, err)
		}
	}

	session, err := createPowerVSSession(option)
	if err != nil {
		return fmt.Errorf("error creating PowerVS session: %w", err)
	}
	v1, err := createVpcService(option)
	if err != nil {
		return fmt.Errorf("error creating VPC service: %w", err)
	}

	err = setupInfra(option, session, v1)
	if err != nil {
		return err
	}

	return nil
}

func setupInfra(option *CreateInfraOptions, session *ibmpisession.IBMPISession, vpc1 *vpcv1.VpcV1) error {
	err := setupPowerVsSubnet(option, session)
	if err != nil {
		return fmt.Errorf("error setup powervs subnet: %w", err)
	}
	return nil
}

func getIAMAuth() *core.IamAuthenticator {
	return &core.IamAuthenticator{
		ApiKey: os.Getenv("IBMCLOUD_API_KEY"),
	}
}

func getServicePlanID(options *CreateInfraOptions) (string, error) {
	gcv1, err := globalcatalogv1.NewGlobalCatalogV1(&globalcatalogv1.GlobalCatalogV1Options{Authenticator: getIAMAuth()})
	if err != nil {
		return "", err
	}

	include := "*"
	listCatalogEntriesOpt := globalcatalogv1.ListCatalogEntriesOptions{Include: &include, Q: &options.ServiceName}
	catalogEntriesList, _, err := gcv1.ListCatalogEntries(&listCatalogEntriesOpt)
	if err != nil {
		return "", err
	}
	var serviceID string
	for _, catalog := range catalogEntriesList.Resources {
		if *catalog.Name == options.ServiceName {
			serviceID = *catalog.ID
		}
	}

	kind := "plan"
	getChildOpt := globalcatalogv1.GetChildObjectsOptions{ID: &serviceID, Kind: &kind}
	childObjResult, _, err := gcv1.GetChildObjects(&getChildOpt)
	if err != nil {
		return "", err
	}
	for _, plan := range childObjResult.Resources {
		if *plan.Name == options.ServicePlanName {
			return *plan.ID, nil
		}
	}

	return "", fmt.Errorf("could not retrieve plan id for service name: %s & service plan name: %s", options.ServiceName, options.ServicePlanName)
}

func getResourceGroupID(options *CreateInfraOptions) (string, error) {
	rmv2, err := resourcemanagerv2.NewResourceManagerV2(&resourcemanagerv2.ResourceManagerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return "", err
	}

	rmv2ListResourceGroupOpt := resourcemanagerv2.ListResourceGroupsOptions{Name: &options.ResourceGroup}
	resourceGroupListResult, _, err := rmv2.ListResourceGroups(&rmv2ListResourceGroupOpt)
	if err != nil {
		return "", err
	}

	for _, rg := range resourceGroupListResult.Resources {
		if *rg.Name == options.ResourceGroup {
			return *rg.ID, nil
		}
	}

	return "", fmt.Errorf("could not retrieve resource group id for %s", options.ResourceGroup)
}

func createCloudInstance(options *CreateInfraOptions) (string, error) {
	resourceGroupID, err := getResourceGroupID(options)
	if err != nil {
		return "", err
	}

	rcv2, err := resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return "", err
	}

	cloudInstanceName := fmt.Sprintf("%s-hypershift-nodepool", options.InfraID)
	servicePlanID, err := getServicePlanID(options)
	if err != nil {
		return "", err
	}

	target := options.PowerVSZone
	resourceInstanceOpt := resourcecontrollerv2.CreateResourceInstanceOptions{Name: &cloudInstanceName,
		ResourceGroup:  &resourceGroupID,
		ResourcePlanID: &servicePlanID,
		Target:         &target}

	resourceInstance, _, err := rcv2.CreateResourceInstance(&resourceInstanceOpt)

	return *resourceInstance.GUID, err
}

func createPowerVSSession(option *CreateInfraOptions) (*ibmpisession.IBMPISession, error) {
	auth := getIAMAuth()
	account, err := servicesUtils.GetAccount(auth)

	if err != nil {
		return nil, fmt.Errorf("error retrieving account: %w", err)
	}

	if option.PowerVSRegion == "" {
		option.PowerVSRegion, err = powerUtils.GetRegion(option.PowerVSZone)
		if err != nil {
			return nil, fmt.Errorf("failed to get region for cloud instance %s, error: %w", option.PowerVSCloudInstanceID, err)
		}
	}

	opt := &ibmpisession.IBMPIOptions{Authenticator: auth,
		Debug:       true,
		Region:      option.PowerVSRegion,
		UserAccount: account,
		Zone:        option.PowerVSZone}

	session, err := ibmpisession.NewIBMPISession(opt)
	return session, err
}

func createVpcService(createOpt *CreateInfraOptions) (*vpcv1.VpcV1, error) {
	v1, err := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: getIAMAuth(),
		URL:           fmt.Sprintf("https://%s.iaas.cloud.ibm.com/v1", createOpt.VpcRegion),
	})
	return v1, err
}

func setupPowerVsInstance(option *CreateInfraOptions, session *ibmpisession.IBMPISession) error {
	if len(option.PowerVSInstanceID) > 0 {

	}
	return nil
}

func setupPowerVsSubnet(option *CreateInfraOptions, session *ibmpisession.IBMPISession) error {
	client := instance.NewIBMPINetworkClient(context.Background(), session, option.PowerVSCloudInstanceID)

	if option.PowerVSSubnetID != "" {
		err := validatePowerVsSubnet(option.PowerVSSubnetID, client)
		if err != nil {
			return err
		}
	} else {
		var err error
		option.PowerVSSubnetID, err = createPowerVsSubnet(client)
		if err != nil {
			return err
		}
	}

	return nil
}

func generateIPData(cidr string) (string, string, string, error) {
	_, ipv4Net, err := net.ParseCIDR(cidr)

	if err != nil {
		return "", "", "", err
	}

	var subnetToSize = map[string]int{
		"21": 2048,
		"22": 1024,
		"23": 512,
		"24": 256,
		"25": 128,
		"26": 64,
		"27": 32,
		"28": 16,
		"29": 8,
		"30": 4,
		"31": 2,
	}
	gateway, err := cidrutil.Host(ipv4Net, 1)
	if err != nil {
		return "", "", "", err
	}
	ad := cidrutil.AddressCount(ipv4Net)

	convertedad := strconv.FormatUint(ad, 10)
	// Powervc in wdc04 has to reserve 3 ip address hence we start from the 4th. This will be the default behaviour
	firstusable, err := cidrutil.Host(ipv4Net, 2)
	if err != nil {

	}
	lastusable, err := cidrutil.Host(ipv4Net, subnetToSize[convertedad]-2)
	if err != nil {
		return "", "", "", err
	}

	return gateway.String(), firstusable.String(), lastusable.String(), nil
}

func findNextNwToUse(client *instance.IBMPINetworkClient) (string, error) {
	networks, err := client.GetAll()
	if err != nil {
		return "", err
	}

	baseCidrSplit := strings.Split(basePowerVsPrivateSubnetCIDR, ".")[:3]
	baseCidrPrefix := strings.Join(baseCidrSplit, ".")
	lastNetwork := basePowerVsPrivateSubnetCIDR
	for _, nw := range networks.Networks {
		network, err := client.Get(*nw.NetworkID)
		if err != nil {
			return "", err
		}
		cidr := *network.Cidr
		if strings.HasPrefix(cidr, baseCidrPrefix) {
			if cidr > lastNetwork {
				lastNetwork = cidr
			}
		}
	}

	lnSplit := strings.Split(lastNetwork, ".")
	lastNwOctS := strings.Split(lnSplit[len(lnSplit)-1], "/")
	lastNwOct, _ := strconv.Atoi(lastNwOctS[0])
	nextOct := lastNwOct + 1
	lastNwOctS[0] = strconv.Itoa(nextOct)
	lnSplit[len(lnSplit)-1] = strings.Join(lastNwOctS, "/")
	nextNw := strings.Join(lnSplit, ".")
	return nextNw, nil
}

func createPowerVsSubnet(client *instance.IBMPINetworkClient) (string, error) {
	dnsServers := make([]string, 1)
	netType := "vlan"           // private network
	dnsServers[0] = "127.0.0.1" // dns servers should point to localhost for private network.

	nextNw, err := findNextNwToUse(client)
	if err != nil {
		return "", err
	}
	gateway, startIP, endIP, err := generateIPData(nextNw)
	if err != nil {
		return "", err
	}

	networkPayload := models.NetworkCreate{Cidr: nextNw,
		DNSServers: dnsServers,
		Gateway:    gateway,
		IPAddressRanges: []*models.IPAddressRange{
			{EndingIPAddress: &endIP, StartingIPAddress: &startIP}},
		Jumbo: false,
		Name:  fmt.Sprintf("%s-hypershift-private-subnet"),
		Type:  &netType,
	}

	network, err := client.Create(&networkPayload)

	if err != nil {
		return "", err
	}
	return *network.NetworkID, nil
}
