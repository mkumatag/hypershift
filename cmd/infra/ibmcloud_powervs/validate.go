package ibmcloud_powervs

import (
	"fmt"
	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/IBM/vpc-go-sdk/vpcv1"
)

func validateCloudInstance(cloudInstance string) (resourceInstance *resourcecontrollerv2.ResourceInstance, err error) {
	rcv2, err := resourcecontrollerv2.NewResourceControllerV2(&resourcecontrollerv2.ResourceControllerV2Options{Authenticator: getIAMAuth()})
	if err != nil {
		return nil, err
	}

	resourceInstanceL, _, err := rcv2.ListResourceInstances(&resourcecontrollerv2.ListResourceInstancesOptions{Name: &cloudInstance})
	if err != nil {
		return nil, err
	}

	for _, resource := range resourceInstanceL.Resources {
		if *resource.Name == cloudInstance {
			resourceInstance = &resource
			break
		}
	}
	if resourceInstance != nil && *resourceInstance.State != "active" {
		return nil, fmt.Errorf("provided cloud instance id is not in active state, current state: %s", *resourceInstance.State)
	}
	return resourceInstance, err
}

func validatePowerVsSubnet(subnetName string, client *instance.IBMPINetworkClient) (*models.Network, error) {
	subnets, err := client.GetAll()
	if err != nil {
		return nil, err
	}

	var subnetRef *models.NetworkReference
	for _, sn := range subnets.Networks {
		if *sn.Name == subnetName {
			subnetRef = sn
		}
	}

	if subnetRef == nil {
		return nil, fmt.Errorf("%s powervs subnet not found", subnetName)
	}
	if subnetRef != nil && *subnetRef.Type != "vlan" {
		return nil, fmt.Errorf("%s powervs subnet is not private", subnetName)
	}

	subnet, err := client.Get(*subnetRef.NetworkID)

	return subnet, err
}

func validateVpc(options *CreateInfraOptions, resourceGroupID string, v1 *vpcv1.VpcV1) (vpc *vpcv1.VPC, err error) {
	vpcList, _, err := v1.ListVpcs(&vpcv1.ListVpcsOptions{ResourceGroupID: &resourceGroupID})
	if err != nil {
		return nil, err
	}
	for _, vpc := range vpcList.Vpcs {
		if *vpc.Name == options.Vpc {
			return &vpc, nil
		}
	}
	return nil, fmt.Errorf("%s vpc not found", options.Vpc)
}

func validateVpcSubnet(option *CreateInfraOptions, resourceGroupID string, v1 *vpcv1.VpcV1) (vpcSubnet *vpcv1.Subnet, err error) {
	subnetList, _, err := v1.ListSubnets(&vpcv1.ListSubnetsOptions{ResourceGroupID: &resourceGroupID})
	if err != nil {
		return nil, err
	}
	for _, subnet := range subnetList.Subnets {
		if *subnet.Name == option.VpcSubnet && *subnet.VPC.Name == option.Vpc {
			return &subnet, nil
		}
	}
	return vpcSubnet, fmt.Errorf("%s vpc subnet not found", option.VpcSubnet)
}

func validateVpcLoadBalancer(option *CreateInfraOptions, v1 *vpcv1.VpcV1) (vpcLoadBalancer *vpcv1.LoadBalancer, err error) {
	loadBalancerList, _, err := v1.ListLoadBalancers(&vpcv1.ListLoadBalancersOptions{})
	if err != nil {
		return nil, err
	}

	for _, loadBalancer := range loadBalancerList.LoadBalancers {
		if *loadBalancer.Name == option.VpcLoadBalancer {
			return &loadBalancer, nil
		}
	}
	return nil, fmt.Errorf("%s load balancer not found", option.VpcLoadBalancer)
}

func validateCloudConnection(option *CreateInfraOptions, client *instance.IBMPICloudConnectionClient) (cloudConn *models.CloudConnection, err error) {
	cloudConnL, err := client.GetAll()
	if err != nil {
		return nil, err
	}
	for _, cc := range cloudConnL.CloudConnections {
		if *cc.Name == option.PowerVSCloudConnection {
			tmp := *cc
			return &tmp, nil
		}
	}
	return nil, fmt.Errorf("%s cloud connection not found", option.PowerVSCloudConnection)
}

func validateDhcpServer(option *CreateInfraOptions, client *instance.IBMPIDhcpClient) (dhcpServer *models.DHCPServerDetail, err error) {
	dhcpServer, err = client.Get(option.PowerVSDhcpServerID)
	return dhcpServer, nil
}
