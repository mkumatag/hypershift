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
	cidrUtil "github.com/apparentlymart/go-cidr/cidr"
	"github.com/openshift/hypershift/cmd/log"
	powerUtils "github.com/ppc64le-cloud/powervs-utils"
	"github.com/spf13/cobra"
	"net"
	"os"
	servicesUtils "sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/utils"
	"time"
)

type CreateInfraOptions struct {
	Service                string
	ServicePlan            string
	ResourceGroup          string
	InfraID                string
	NodePoolReplicas       int
	PowerVSRegion          string
	PowerVSZone            string
	PowerVSCloudInstance   string
	PowerVSSubnet          string
	PowerVSCloudConnection string
	VpcRegion              string
	Vpc                    string
	VpcSubnet              string
	VpcLoadBalancer        string
}

const (
	basePowerVsPrivateSubnetCIDR = "10.0.1.0/24"
	jobCompleted                 = "completed"
	jobFailed                    = "failed"
)

type Infra struct {
	ResourceGroupID          string `json:"resourceGroupID"`
	PowerVSCloudInstanceID   string `json:"powerVSCloudInstanceID"`
	PowerVSSubnetID          string `json:"powerVSSubnetID"`
	PowerVSCloudConnectionID string `json:"powerVSCloudConnectionID"`
	VpcID                    string `json:"vpcID"`
	VpcSubnetID              string `json:"vpcSubnetID"`
	VpcLoadBalancerID        string `json:"vpcLoadBalancerID"`
}

func NewCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "powervs",
		Short:        "Creates PowerVS infrastructure resources for a cluster",
		SilenceUsage: true,
	}

	opts := CreateInfraOptions{Service: "power-iaas", ServicePlan: "power-virtual-server-group"}

	cmd.Flags().StringVar(&opts.ResourceGroup, "resource-group", opts.ResourceGroup, "IBM Cloud Resource Group")
	cmd.Flags().StringVar(&opts.InfraID, "infra-id", opts.InfraID, "Cluster ID with which to tag IBM Cloud resources")
	cmd.Flags().IntVar(&opts.NodePoolReplicas, "node-pool-replicas", 1, "If >-1, create a default NodePool with this many replicas")
	cmd.Flags().StringVar(&opts.PowerVSRegion, "pv-region", opts.PowerVSRegion, "IBM Cloud PowerVS Region")
	cmd.Flags().StringVar(&opts.PowerVSZone, "pv-zone", opts.PowerVSZone, "PowerVS Region's Zone")
	cmd.Flags().StringVar(&opts.PowerVSCloudInstance, "pv-cloud-instance", opts.PowerVSCloudInstance, "IBM PowerVS Cloud Instance Name")
	cmd.Flags().StringVar(&opts.PowerVSSubnet, "pv-subnet", opts.PowerVSSubnet, "PowerVS Private Subnet Name")
	cmd.Flags().StringVar(&opts.VpcRegion, "vpc-region", opts.VpcRegion, "IBM Cloud VPC Region for VPC resources")
	cmd.Flags().StringVar(&opts.Vpc, "vpc", opts.Vpc, "IBM Cloud VPC Name")
	cmd.Flags().StringVar(&opts.VpcSubnet, "vpc-subnet", opts.VpcSubnet, "VPC Subnet Name")
	cmd.Flags().StringVar(&opts.VpcLoadBalancer, "vpc-load-balancer", opts.VpcLoadBalancer, "VPC Load Balancer Name")
	cmd.Flags().StringVar(&opts.PowerVSCloudConnection, "pv-cloud-connection-id", opts.PowerVSCloudConnection, "IBM Cloud PowerVS Cloud Connection ")

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

	session, err := createPowerVSSession(option)
	if err != nil {
		return fmt.Errorf("error creating PowerVS session: %w", err)
	}
	v1, err := createVpcService(option)
	if err != nil {
		return fmt.Errorf("error creating VPC service: %w", err)
	}

	infra := &Infra{}
	err = infra.setupInfra(option, session, v1)
	if err != nil {
		return err
	}

	return nil
}

func (infra *Infra) setupInfra(option *CreateInfraOptions, session *ibmpisession.IBMPISession, v1 *vpcv1.VpcV1) (err error) {
	err = infra.setupPowerVsCloudInstance(option)
	if err != nil {
		return fmt.Errorf("error setup powervs cloud instance: %w", err)
	}

	err = infra.setupPowerVsSubnet(option, session)
	if err != nil {
		return fmt.Errorf("error setup powervs subnet: %w", err)
	}

	err = infra.setupVpc(option, v1)
	if err != nil {
		return fmt.Errorf("error setup vpc: %w", err)
	}

	err = infra.setupVpcSubnet(option, v1)
	if err != nil {
		return fmt.Errorf("error setup vpc subnet: %w", err)
	}

	err = infra.setupVpcLoadBalancer(option, v1)
	if err != nil {
		return fmt.Errorf("error setup vpc load balancer: %w", err)
	}

	err = infra.setupPowerVsCloudConnection(option, session)
	return nil
}

func getIAMAuth() *core.IamAuthenticator {
	return &core.IamAuthenticator{
		ApiKey: os.Getenv("IBMCLOUD_API_KEY"),
	}
}

func getServicePlanID(option *CreateInfraOptions) (servicePlanID string, err error) {
	gcv1, err := globalcatalogv1.NewGlobalCatalogV1(&globalcatalogv1.GlobalCatalogV1Options{Authenticator: getIAMAuth()})
	if err != nil {
		return "", err
	}

	include := "*"
	listCatalogEntriesOpt := globalcatalogv1.ListCatalogEntriesOptions{Include: &include, Q: &option.Service}
	catalogEntriesList, _, err := gcv1.ListCatalogEntries(&listCatalogEntriesOpt)
	if err != nil {
		return "", err
	}
	var serviceID string
	for _, catalog := range catalogEntriesList.Resources {
		if *catalog.Name == option.Service {
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
		if *plan.Name == option.ServicePlan {
			return *plan.ID, nil
		}
	}

	return "", fmt.Errorf("could not retrieve plan id for service name: %s & service plan name: %s", option.Service, option.ServicePlan)
}

func getResourceGroupID(option *CreateInfraOptions) (resourceGroupID string, err error) {
	rmv2, err := resourcemanagerv2.NewResourceManagerV2(&resourcemanagerv2.ResourceManagerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return "", err
	}

	rmv2ListResourceGroupOpt := resourcemanagerv2.ListResourceGroupsOptions{Name: &option.ResourceGroup}
	resourceGroupListResult, _, err := rmv2.ListResourceGroups(&rmv2ListResourceGroupOpt)
	if err != nil {
		return "", err
	}

	for _, rg := range resourceGroupListResult.Resources {
		if *rg.Name == option.ResourceGroup {
			return *rg.ID, nil
		}
	}

	return "", fmt.Errorf("could not retrieve resource group id for %s", option.ResourceGroup)
}

func (infra *Infra) createCloudInstance(option *CreateInfraOptions) (resourceInstance *resourcecontrollerv2.ResourceInstance, err error) {
	infra.ResourceGroupID, err = getResourceGroupID(option)
	if err != nil {
		return nil, err
	}

	rcv2, err := resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return nil, err
	}

	cloudInstanceName := fmt.Sprintf("%s-hypershift-nodepool", option.InfraID)
	servicePlanID, err := getServicePlanID(option)
	if err != nil {
		return nil, err
	}

	target := option.PowerVSZone
	resourceInstanceOpt := resourcecontrollerv2.CreateResourceInstanceOptions{Name: &cloudInstanceName,
		ResourceGroup:  &infra.ResourceGroupID,
		ResourcePlanID: &servicePlanID,
		Target:         &target}

	resourceInstance, _, err = rcv2.CreateResourceInstance(&resourceInstanceOpt)

	return resourceInstance, err
}

func createPowerVSSession(option *CreateInfraOptions) (session *ibmpisession.IBMPISession, err error) {
	auth := getIAMAuth()
	account, err := servicesUtils.GetAccount(auth)

	if err != nil {
		return nil, fmt.Errorf("error retrieving account: %w", err)
	}

	if option.PowerVSRegion == "" {
		option.PowerVSRegion, err = powerUtils.GetRegion(option.PowerVSZone)
		if err != nil {
			return nil, fmt.Errorf("failed to get region for cloud instance %s, error: %w", option.PowerVSCloudInstance, err)
		}
	}

	opt := &ibmpisession.IBMPIOptions{Authenticator: auth,
		Debug:       true,
		Region:      option.PowerVSRegion,
		UserAccount: account,
		Zone:        option.PowerVSZone}

	session, err = ibmpisession.NewIBMPISession(opt)
	return session, err
}

func createVpcService(createOpt *CreateInfraOptions) (v1 *vpcv1.VpcV1, err error) {
	v1, err = vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: getIAMAuth(),
		URL:           fmt.Sprintf("https://%s.iaas.cloud.ibm.com/v1", createOpt.VpcRegion),
	})
	return v1, err
}

func (infra *Infra) setupPowerVsCloudInstance(option *CreateInfraOptions) (err error) {
	var cloudInstance *resourcecontrollerv2.ResourceInstance
	if option.PowerVSCloudInstance != "" {
		cloudInstance, err = validateCloudInstance(option.PowerVSCloudInstance)
		if err != nil {
			return fmt.Errorf("error validating cloud instance id %s, %w", option.PowerVSCloudInstance, err)
		}
	} else {
		cloudInstance, err = infra.createCloudInstance(option)
		if err != nil {
			return fmt.Errorf("error creating cloud instance: %w", err)
		}
		log.Log.Info("Cloud Instance Created", "cloudInstanceID", *cloudInstance.ID)
	}

	if cloudInstance != nil {
		infra.PowerVSCloudInstanceID = *cloudInstance.ID
	}

	return nil
}

func (infra *Infra) setupPowerVsSubnet(option *CreateInfraOptions, session *ibmpisession.IBMPISession) (err error) {
	client := instance.NewIBMPINetworkClient(context.Background(), session, infra.PowerVSCloudInstanceID)

	var network *models.Network
	if option.PowerVSSubnet != "" {
		network, err = validatePowerVsSubnet(option.PowerVSSubnet, client)
		if err != nil {
			return err
		}
		availableNet := int(*network.IPAddressMetrics.Available)
		if availableNet < option.NodePoolReplicas {
			return fmt.Errorf("given network %s, does not accommodate %d node pool replicas", option.PowerVSSubnet, option.NodePoolReplicas)
		}
	} else {
		network, err = createPowerVsSubnet(option, client, basePowerVsPrivateSubnetCIDR)
		if err != nil {
			return err
		}
	}

	if network != nil {
		infra.PowerVSSubnetID = *network.NetworkID
	}

	return nil
}

func createPowerVsSubnet(option *CreateInfraOptions, client *instance.IBMPINetworkClient, cidr string) (network *models.Network, err error) {
	dnsServers := make([]string, 1)
	netType := "vlan"           // private network
	dnsServers[0] = "127.0.0.1" // dns servers should point to localhost for private network.

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("error parsing default subnet CIDR: %w", err)
	}
	gateway, err := cidrUtil.Host(ipNet, 1)

	networkPayload := models.NetworkCreate{Cidr: cidr,
		DNSServers: dnsServers,
		Gateway:    gateway.String(),
		Jumbo:      false,
		Name:       fmt.Sprintf("%s-hypershift-private-subnet", option.InfraID),
		Type:       &netType,
	}

	network, err = client.Create(&networkPayload)

	if err != nil {
		return nil, err
	}
	return network, nil
}

func (infra *Infra) setupVpc(option *CreateInfraOptions, v1 *vpcv1.VpcV1) (err error) {
	var vpc *vpcv1.VPC
	if option.Vpc != "" {
		vpc, err = validateVpc(option, infra.ResourceGroupID, v1)
		if err != nil {
			return err
		}
	} else {
		vpc, err = createVpc(option, infra.ResourceGroupID, v1)
		if err != nil {
			return err
		}
	}

	if vpc != nil {
		infra.VpcID = *vpc.ID
	}
	return nil
}

func createVpc(option *CreateInfraOptions, resourceGroupID string, v1 *vpcv1.VpcV1) (vpc *vpcv1.VPC, err error) {
	vpcName := fmt.Sprintf("%s-hypershift-vpc", option.InfraID)
	addressPrefixManagement := "auto"
	classicAccess := false
	vpcOption := &vpcv1.CreateVPCOptions{
		ResourceGroup:           &vpcv1.ResourceGroupIdentity{ID: &resourceGroupID},
		Name:                    &vpcName,
		AddressPrefixManagement: &addressPrefixManagement,
		ClassicAccess:           &classicAccess,
	}
	vpc, _, err = v1.CreateVPC(vpcOption)
	return vpc, err
}

func (infra *Infra) setupVpcSubnet(option *CreateInfraOptions, v1 *vpcv1.VpcV1) (err error) {
	if option.Vpc == "" {
		// VPC created by this script, where AddressPrefixManagement set to 'auto', leads to creating subnets in all zone
		vpcSubnets, _, err := v1.ListSubnets(&vpcv1.ListSubnetsOptions{ResourceGroupID: &infra.ResourceGroupID})
		if err != nil {
			return fmt.Errorf("error listing subnets in resource group: %s, %w", option.ResourceGroup, err)
		}
		for _, subnet := range vpcSubnets.Subnets {
			if *subnet.VPC.ID == infra.VpcID {
				infra.VpcSubnetID = *subnet.ID
				break
			}
		}
		return nil
	} else {
		vpcSubnet, err := validateVpcSubnet(option, infra.ResourceGroupID, v1)
		if err != nil {
			return err
		}
		infra.VpcSubnetID = *vpcSubnet.ID
	}
	return nil
}

func (infra *Infra) setupVpcLoadBalancer(option *CreateInfraOptions, v1 *vpcv1.VpcV1) (err error) {
	var loadBalancer *vpcv1.LoadBalancer
	if option.VpcLoadBalancer != "" {
		loadBalancer, err = validateVpcLoadBalancer(option, v1)
		if err != nil {
			return err
		}
	} else {
		loadBalancer, err = infra.createVpcLoadBalancer(option, v1)
		if err != nil {
			return err
		}
	}

	if loadBalancer != nil {
		infra.VpcLoadBalancerID = *loadBalancer.ID
	}
	return nil
}

func (infra *Infra) createVpcLoadBalancer(option *CreateInfraOptions, v1 *vpcv1.VpcV1) (loadBalancer *vpcv1.LoadBalancer, err error) {
	loadBalancerName := fmt.Sprintf("%s-hypershift-load-balancer", option.InfraID)
	isPublicLoadBalancer := false
	subnets := []vpcv1.SubnetIdentityIntf{&vpcv1.SubnetIdentity{ID: &infra.VpcSubnetID}}
	var resourceGroupIdentity vpcv1.ResourceGroupIdentityIntf = &vpcv1.ResourceGroupIdentityByID{&infra.ResourceGroupID}

	loadBalancerCreateOption := &vpcv1.CreateLoadBalancerOptions{
		Name:          &loadBalancerName,
		IsPublic:      &isPublicLoadBalancer,
		Subnets:       subnets,
		ResourceGroup: resourceGroupIdentity,
	}

	loadBalancer, _, err = v1.CreateLoadBalancer(loadBalancerCreateOption)
	return loadBalancer, err
}

func (infra *Infra) setupPowerVsCloudConnection(option *CreateInfraOptions, session *ibmpisession.IBMPISession) (err error) {
	client := instance.NewIBMPICloudConnectionClient(context.Background(), session, infra.PowerVSCloudInstanceID)
	jobClient := instance.NewIBMPIJobClient(context.Background(), session, infra.PowerVSCloudInstanceID)
	if option.PowerVSCloudConnection != "" {
		cloudConn, err := validateCloudConnection(option, client)
		if err != nil {
			return err
		}

		if option.PowerVSSubnet != "" && *cloudConn.Networks[0].NetworkID != infra.PowerVSSubnetID {
			return fmt.Errorf("cloud connection %s does not contain powervs subnet %s", option.PowerVSCloudConnection, option.PowerVSSubnet)
		} else {
			_, job, err := client.AddNetwork(*cloudConn.CloudConnectionID, infra.PowerVSSubnetID)
			if err != nil {
				return fmt.Errorf("%s while adding powervs subsnet to %s cloud connection", err, option.PowerVSCloudConnection)
			}
			j, err := waitForJobCompletion(job, jobClient)
			if err != nil {
				return fmt.Errorf("%s while adding powervs subsnet to %s cloud connection", err, option.PowerVSCloudConnection)
			}
			if *j.Status.State == jobFailed {
				return fmt.Errorf("%s while adding powervs subsnet to %s cloud connection", j.Status.Message, option.PowerVSCloudConnection)
			}
		}

		if option.Vpc != "" && *cloudConn.Vpc.Vpcs[0].VpcID != infra.VpcID {
			return fmt.Errorf("cloud connection %s does not contain vpc %s", option.PowerVSCloudConnection, option.Vpc)
		} else {
			vpc := []*models.CloudConnectionVPC{&models.CloudConnectionVPC{VpcID: &infra.VpcID}}
			_, job, err := client.Update(*cloudConn.CloudConnectionID, &models.CloudConnectionUpdate{
				Vpc: &models.CloudConnectionEndpointVPC{
					Vpcs: vpc},
			})
			if err != nil {
				return fmt.Errorf("%s while adding vpc to %s cloud connection", err, option.PowerVSCloudConnection)
			}
			j, err := waitForJobCompletion(job, jobClient)
			if err != nil {
				return fmt.Errorf("%s while adding vpc to %s cloud connection", err, option.PowerVSCloudConnection)
			}
			if *j.Status.State == jobFailed {
				return fmt.Errorf("%s while adding vpc to %s cloud connection", j.Status.Message, option.PowerVSCloudConnection)
			}
		}
	}

	return nil
}

func waitForJobCompletion(jobToMonitor *models.JobReference, client *instance.IBMPIJobClient) (job *models.Job, err error) {
	var status string
	for status != jobCompleted && status != jobFailed {
		job, err = client.Get(*jobToMonitor.ID)
		if err != nil {
			return nil, err
		}
		if job == nil || job.Status == nil {
			return nil, fmt.Errorf("cannot find job status for job id %s", *jobToMonitor.ID)
		}
		time.Sleep(2000)
		status = *job.Status.State
	}
	return job, err
}
