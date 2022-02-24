package ibmcloud_powervs

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/networking-go-sdk/zonesv1"
	"github.com/IBM/platform-services-go-sdk/globalcatalogv1"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/platform-services-go-sdk/resourcemanagerv2"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	"github.com/openshift/hypershift/cmd/log"
	powerUtils "github.com/ppc64le-cloud/powervs-utils"
	"github.com/spf13/cobra"
	"os"
	servicesUtils "sigs.k8s.io/cluster-api-provider-ibmcloud/pkg/cloud/services/utils"
	"strings"
	"time"
)

type CreateInfraOptions struct {
	CISInstance            string
	BaseDomain             string
	Service                string
	ServicePlan            string
	ResourceGroup          string
	InfraID                string
	NodePoolReplicas       int
	PowerVSRegion          string
	PowerVSZone            string
	PowerVSCloudInstance   string
	PowerVSDhcpServerID    string
	PowerVSDhcpSubnet      string
	PowerVSCloudConnection string
	VpcRegion              string
	Vpc                    string
	VpcSubnet              string
	VpcLoadBalancer        string
	OutputFile             string
}

const (
	defaultCloudConnSpeed = 5000
)

type Infra struct {
	CisCrn                   string `json:"cisCrn"`
	CisDomainID              string `json:"cisDomainID"`
	ResourceGroupID          string `json:"resourceGroupID"`
	PowerVSCloudInstanceID   string `json:"powerVSCloudInstanceID"`
	PowerVSDhcpSubnetID      string `json:"powerVSDhcpSubnetID"`
	PowerVSDhcpID            string `json:"powerVSDhcpID"`
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

	cmd.Flags().StringVar(&opts.CISInstance, "cis-instance", opts.CISInstance, "IBM Cloud CIS Instance")
	cmd.Flags().StringVar(&opts.BaseDomain, "base-domain", opts.BaseDomain, "IBM Cloud CIS Domain")
	cmd.Flags().StringVar(&opts.ResourceGroup, "resource-group", opts.ResourceGroup, "IBM Cloud Resource Group")
	cmd.Flags().StringVar(&opts.InfraID, "infra-id", opts.InfraID, "Cluster ID with which to tag IBM Cloud resources")
	cmd.Flags().IntVar(&opts.NodePoolReplicas, "node-pool-replicas", 1, "If >-1, create a default NodePool with this many replicas")
	cmd.Flags().StringVar(&opts.PowerVSRegion, "pv-region", opts.PowerVSRegion, "IBM Cloud PowerVS Region")
	cmd.Flags().StringVar(&opts.PowerVSZone, "pv-zone", opts.PowerVSZone, "PowerVS Region's Zone")
	cmd.Flags().StringVar(&opts.PowerVSCloudInstance, "pv-cloud-instance", opts.PowerVSCloudInstance, "IBM PowerVS Cloud Instance Name")
	cmd.Flags().StringVar(&opts.PowerVSDhcpServerID, "pv-dhcp-server-id", opts.PowerVSDhcpServerID, "PowerVS DHCP Server ID")
	cmd.Flags().StringVar(&opts.PowerVSDhcpSubnet, "pv-dhcp-subnet", opts.PowerVSDhcpSubnet, "PowerVS DHCP Private Subnet Name")
	cmd.Flags().StringVar(&opts.VpcRegion, "vpc-region", opts.VpcRegion, "IBM Cloud VPC Region for VPC resources. "+
		"\nPossible Values:\n 1. Dallas\n 2. Frankfurt\n 3. London\n 4. Osaka\n 5. Sao Palo\n 6. Sydney\n 7. Tokyo\n 8. Toronto\n 9. Washington DC\n")
	cmd.Flags().StringVar(&opts.Vpc, "vpc", opts.Vpc, "IBM Cloud VPC Name")
	cmd.Flags().StringVar(&opts.VpcSubnet, "vpc-subnet", opts.VpcSubnet, "VPC Subnet Name")
	cmd.Flags().StringVar(&opts.VpcLoadBalancer, "vpc-load-balancer", opts.VpcLoadBalancer, "VPC Load Balancer Name")
	cmd.Flags().StringVar(&opts.PowerVSCloudConnection, "pv-cloud-connection", opts.PowerVSCloudConnection, "IBM Cloud PowerVS Cloud Connection ")
	cmd.Flags().StringVar(&opts.OutputFile, "output-file", opts.OutputFile, "Path to file that will contain output information from infra resources (optional)")

	cmd.MarkFlagRequired("cis-instance")
	cmd.MarkFlagRequired("base-domain")
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
	infra := &Infra{}
	err = infra.setupInfra(option)
	if err != nil {
		return err
	}

	return nil
}

func (infra *Infra) setupInfra(option *CreateInfraOptions) (err error) {
	defer func() {
		out := os.Stdout
		if len(option.OutputFile) > 0 {
			var err error
			out, err = os.Create(option.OutputFile)
			if err != nil {
				log.Log.Error(err, "cannot create output file")
			}
			defer out.Close()
		}
		outputBytes, err := json.MarshalIndent(infra, "", "  ")
		if err != nil {
			log.Log.Error(err, "failed to serialize result")
		}
		_, err = out.Write(outputBytes)
		if err != nil {
			log.Log.Error(err, "failed to write result")
		}
	}()

	log.Log.Info("Setup Infra started")
	err = infra.setupBaseDomain(option)
	if err != nil {
		return fmt.Errorf("error setup base domain: %w", err)
	}

	v1, err := createVpcService(option)
	if err != nil {
		return fmt.Errorf("error creating VPC service: %w", err)
	}

	err = infra.setupVpc(option, v1)
	if err != nil {
		return fmt.Errorf("error setup vpc: %w", err)
	}

	err = infra.setupVpcSubnet(option, v1)
	if err != nil {
		return fmt.Errorf("error setup vpc subnet: %w", err)
	}

	session, err := createPowerVSSession(option)
	if err != nil {
		return fmt.Errorf("error creating PowerVS session: %w", err)
	}

	err = infra.setupPowerVsCloudInstance(option)
	if err != nil {
		return fmt.Errorf("error setup powervs cloud instance: %w", err)
	}

	err = infra.setupPowerVsCloudConnection(option, session)
	if err != nil {
		return fmt.Errorf("error setup powervs cloud connection: %w", err)
	}

	err = infra.setupPowerVsDhcp(option, session)
	if err != nil {
		return fmt.Errorf("error setup powervs dhcp server: %w", err)
	}

	return nil
}

func getIAMAuth() *core.IamAuthenticator {
	return &core.IamAuthenticator{
		ApiKey: os.Getenv("IBMCLOUD_API_KEY"),
	}
}

func (infra *Infra) setupBaseDomain(option *CreateInfraOptions) (err error) {
	rcv2, err := resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return err
	}

	if rcv2 == nil {
		return fmt.Errorf("Unable to get NewResourceControllerV2")
	}

	var cisInstance *resourcecontrollerv2.ResourceInstance
	resourceList, _, err := rcv2.ListResourceInstances(&resourcecontrollerv2.ListResourceInstancesOptions{Name: &option.CISInstance})
	if err != nil {
		return err
	}
	if resourceList != nil {
		for _, resource := range resourceList.Resources {
			if *resource.Name == option.CISInstance {
				cisInstance = &resource
			}
		}
	}

	if cisInstance == nil {
		return fmt.Errorf("Unable to get CIS Instance")
	}

	zv1, err := zonesv1.NewZonesV1(&zonesv1.ZonesV1Options{Authenticator: getIAMAuth(), Crn: cisInstance.CRN})
	if err != nil {
		return err
	}

	if zv1 != nil {
		return fmt.Errorf("Unable to get NewZonesV1")
	}

	zoneList, _, err := zv1.ListZones(&zonesv1.ListZonesOptions{})
	if err != nil {
		return err
	}

	if zoneList != nil {
		for _, zone := range zoneList.Result {
			if *zone.Name == option.BaseDomain {
				infra.CisCrn = *cisInstance.CRN
				infra.CisDomainID = *zone.ID
			}
		}
	}

	if infra.CisCrn == "" || infra.CisDomainID == "" {
		return fmt.Errorf("Unable to get CIS Information")
	}

	log.Log.Info("BaseDomain Info Ready", "CRN", infra.CisCrn, "DomainID", infra.CisDomainID)
	return nil
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
	endPointUrl, err := getVpcUrl(strings.ToLower(createOpt.VpcRegion))
	v1, err = vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: getIAMAuth(),
		URL:           fmt.Sprintf("https://%s.iaas.cloud.ibm.com/v1", endPointUrl),
	})
	return v1, err
}

func getVpcUrl(region string) (url string, err error) {
	switch {
	case strings.Contains(region, "dallas"), strings.HasPrefix(region, "us-south"):
		url = "us-south"
	case strings.Contains(region, "frankfurt"):
		url = "eu-de"
	case strings.Contains(region, "london"):
		url = "eu-gb"
	case strings.Contains(region, "osaka"):
		url = "jp-osa"
	case strings.Contains(region, "sao"):
		url = "br-sao"
	case strings.Contains(region, "sydney"):
		url = "au-syd"
	case strings.Contains(region, "tokyo"):
		url = "jp-tok"
	case strings.Contains(region, "toronto"):
		url = "ca-tor"
	case strings.Contains(region, "washington"), strings.Contains(region, "us-east"):
		url = "us-east"
	default:
		return "", fmt.Errorf("vpc region %s is not supported", region)
	}
	return url, nil
}

func (infra *Infra) setupPowerVsCloudInstance(option *CreateInfraOptions) (err error) {
	var cloudInstance *resourcecontrollerv2.ResourceInstance
	if option.PowerVSCloudInstance != "" {
		log.Log.Info("Validating PowerVS Cloud Instance", "name", option.PowerVSCloudInstance)
		cloudInstance, err = validateCloudInstance(option.PowerVSCloudInstance)
		if err != nil {
			return fmt.Errorf("error validating cloud instance id %s, %w", option.PowerVSCloudInstance, err)
		}
	} else {
		log.Log.Info("Creating PowerVS Cloud Instance ...")
		cloudInstance, err = infra.createCloudInstance(option)
		if err != nil {
			return fmt.Errorf("error creating cloud instance: %w", err)
		}
	}

	if cloudInstance != nil {
		infra.PowerVSCloudInstanceID = *cloudInstance.GUID
	}

	if infra.PowerVSCloudInstanceID == "" {
		return fmt.Errorf("Unable to setup PowerVS Cloud Instance")
	}

	log.Log.Info("PowerVS Cloud Instance Ready", "id", infra.PowerVSCloudInstanceID)
	return nil
}

func (infra *Infra) setupVpc(option *CreateInfraOptions, v1 *vpcv1.VpcV1) (err error) {
	var vpc *vpcv1.VPC
	if option.Vpc != "" {
		log.Log.Info("Validating VPC", "name", option.Vpc)
		vpc, err = validateVpc(option, infra.ResourceGroupID, v1)
		if err != nil {
			return err
		}
	} else {
		log.Log.Info("Creating VPC ...")
		vpc, err = createVpc(option, infra.ResourceGroupID, v1)
		if err != nil {
			return err
		}
	}

	if vpc != nil {
		infra.VpcID = *vpc.ID
	}

	if infra.VpcID == "" {
		return fmt.Errorf("Unable to setup VPC")
	}

	log.Log.Info("VPC Ready", "ID", infra.VpcID)
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
		// VPC created by hypershift sets AddressPrefixManagement to 'auto', leads to creating subnets in all zones
		vpcSubnets, _, err := v1.ListSubnets(&vpcv1.ListSubnetsOptions{ResourceGroupID: &infra.ResourceGroupID})
		if err != nil {
			return fmt.Errorf("error listing subnets in resource group: %s, %w", option.ResourceGroup, err)
		}
		if vpcSubnets != nil {
			for _, subnet := range vpcSubnets.Subnets {
				if *subnet.VPC.ID == infra.VpcID {
					infra.VpcSubnetID = *subnet.ID
					break
				}
			}
		}
	} else {
		log.Log.Info("Validating VPC Subnet %s", option.VpcSubnet)
		vpcSubnet, err := validateVpcSubnet(option, infra.ResourceGroupID, v1)
		if err != nil {
			return err
		}
		if vpcSubnet != nil {
			infra.VpcSubnetID = *vpcSubnet.ID
		}
	}

	if infra.VpcSubnetID == "" {
		return fmt.Errorf("Unable to setup VPC Subnet")
	}

	log.Log.Info("VPC Subnet Ready", "ID", infra.VpcSubnetID)
	return nil
}

func (infra *Infra) setupPowerVsCloudConnection(option *CreateInfraOptions, session *ibmpisession.IBMPISession) (err error) {
	client := instance.NewIBMPICloudConnectionClient(context.Background(), session, infra.PowerVSCloudInstanceID)
	var cloudConn *models.CloudConnection
	if option.PowerVSCloudConnection != "" {
		log.Log.Info("Validating PowerVS Cloud Connection", "name", option.PowerVSCloudConnection)
		cloudConn, err = validateCloudConnection(option, client)
		if err != nil {
			return err
		}
	} else {
		log.Log.Info("Creating PowerVS Cloud Connection ...")
		cloudConn, err = infra.createCloudConnection(option, client)
		if err != nil {
			return err
		}
	}
	if cloudConn != nil {
		infra.PowerVSCloudConnectionID = *cloudConn.CloudConnectionID
	}

	if infra.PowerVSCloudConnectionID == "" {
		return fmt.Errorf("Unable to setup PowerVS Cloud Connection")
	}

	log.Log.Info("PowerVS Cloud Connection Ready", "id", infra.PowerVSCloudConnectionID)
	return nil
}

func (infra *Infra) createCloudConnection(option *CreateInfraOptions, client *instance.IBMPICloudConnectionClient) (cloudConn *models.CloudConnection, err error) {
	cloudConnName := fmt.Sprintf("%s-hypershift-cloud-connection", option.InfraID)
	var speed int64 = defaultCloudConnSpeed
	var vpcL []*models.CloudConnectionVPC
	vpcId := infra.VpcID
	vpcL = append(vpcL, &models.CloudConnectionVPC{Name: "vpcName", VpcID: &vpcId})

	cloudConnectionEndpointVPC := models.CloudConnectionEndpointVPC{Enabled: true, Vpcs: vpcL}

	cloudConn, _, err = client.Create(&models.CloudConnectionCreate{Name: &cloudConnName, GlobalRouting: true, Speed: &speed, Vpc: &cloudConnectionEndpointVPC})

	if err != nil {
		return nil, err
	}

	if cloudConn == nil {
		return nil, fmt.Errorf("Created cloud connection is nil")
	}

	time.Sleep(10 * time.Second)
	ttl := 0
	for {
		conn, err := client.Get(*cloudConn.CloudConnectionID)
		if err != nil {
			return nil, err
		}

		if conn != nil {
			log.Log.Info("Waiting for Cloud Connection to up", "id", conn.CloudConnectionID, "status", conn.LinkStatus)
			if *conn.LinkStatus == "connect" {
				break
			}
		}
		if ttl > 1800 {
			break
		}
		time.Sleep(5 * time.Second)
		ttl += 5
	}
	return cloudConn, nil
}

func (infra *Infra) setupPowerVsDhcp(option *CreateInfraOptions, session *ibmpisession.IBMPISession) (err error) {
	client := instance.NewIBMPIDhcpClient(context.Background(), session, infra.PowerVSCloudInstanceID)
	var dhcpServer *models.DHCPServerDetail
	if option.PowerVSDhcpServerID != "" {
		log.Log.Info("Validating PowerVS DhcpServer", "id", option.PowerVSDhcpServerID)
		dhcpServer, err = validateDhcpServer(option, client)
		if err != nil {
			return err
		}
	} else {
		log.Log.Info("Creating PowerVS Cloud Connection ...")
		dhcpServer, err = infra.createPowerVsDhcp(client)
		if err != nil {
			return err
		}
	}

	if dhcpServer != nil {
		infra.PowerVSDhcpID = *dhcpServer.ID
		if *dhcpServer.Status == "ACTIVE" && dhcpServer.Network != nil {
			infra.PowerVSDhcpSubnetID = *dhcpServer.Network.ID
		}
	}
	if infra.PowerVSDhcpID == "" && infra.PowerVSDhcpSubnetID == "" {
		return fmt.Errorf("Unable to setup PowerVS DHCP Server and Private Subnet")
	}

	log.Log.Info("PowerVS DHCP Server and Private Subnet  Ready", "dhcpServerId", infra.PowerVSDhcpID, "dhcpPrivateSubnetId", infra.PowerVSDhcpSubnetID)
	return nil
}

func (infra *Infra) createPowerVsDhcp(client *instance.IBMPIDhcpClient) (dhcpServer *models.DHCPServerDetail, err error) {
	dhcp, err := client.Create(&models.DHCPServerCreate{CloudConnectionID: infra.PowerVSCloudConnectionID})
	if err != nil {
		return nil, err
	}

	if dhcp == nil {
		return nil, fmt.Errorf("Created dhcp server is nil")
	}

	time.Sleep(10 * time.Second)
	var server *models.DHCPServerDetail
	ttl := 0
	for {
		server, err = client.Get(*dhcp.ID)
		if err != nil {
			return nil, err
		}

		if server != nil {
			log.Log.Info("Waiting for DhcpServer to up", "id", *server.ID, "status", *server.Status)
			if *server.Status == "ACTIVE" || *server.Status == "FAILED" || *server.Status == "ERROR" {
				break
			}
		}

		if ttl > 1800 {
			break
		}
		time.Sleep(5 * time.Second)
		ttl += 5
	}

	return server, nil
}
