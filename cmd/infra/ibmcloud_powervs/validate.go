package ibmcloud_powervs

/*
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM/vpc-go-sdk/vpcv1"
	"github.com/openshift/hypershift/cmd/log"
	"io/ioutil"
	"strings"
)


func validateManagedInfra(option *CreateInfraOptions, session *ibmpisession.IBMPISession, vpcV1 *vpcv1.VpcV1) error {
	rawJson, err := ioutil.ReadFile(option.ManagedInfraJson)
	if err != nil {
		return fmt.Errorf("failed to read managed infra json: %w", err)
	}

	var managedInfra = &ManagedInfra{}
	if err = json.Unmarshal(rawJson, managedInfra); err != nil {
		return fmt.Errorf("failed to load infra json: %w", err)
	}

	if managedInfra != nil {
		log.Log.Info("managedInfra", "provided:", managedInfra)

		err = managedInfra.validatePowerVSSubnet(session, option)
		if err != nil {
			return fmt.Errorf("error validating powervs subnet: %w", err)
		}

		err = managedInfra.validatePowerVSInstance(session, option)
		if err != nil {
			return fmt.Errorf("error validating powervs instance: %w", err)
		}

		err = managedInfra.validateVpc(vpcV1)
		if err != nil {
			return fmt.Errorf("error validating vpc: %w", err)
		}

		err = managedInfra.validateCloudConnection(session, option)
		if err != nil {
			return fmt.Errorf("error validating cloud connection: %w", err)
		}
	}
	return nil
}

func (managedInfra *ManagedInfra) validatePowerVSSubnet(session *ibmpisession.IBMPISession, option *CreateInfraOptions) error {
	pvNetworkClient := instance.NewIBMPINetworkClient(context.Background(), session, option.CloudInstanceID)
	for _, subnetId := range managedInfra.PowerVSPrivateSubnet {
		subnet, err := pvNetworkClient.Get(subnetId)
		if err != nil {
			return fmt.Errorf("subnet: %s, error: %w", subnetId, err)
		}

		if *subnet.Type != "vlan" {
			return fmt.Errorf("subnet: %s, provided network is not private", subnetId)
		}

		log.Log.Info("validated subnet:", "id", subnetId)
	}
	return nil
}

func validatePowerVSInstance(pvInstances []string, pvSubnet string, session *ibmpisession.IBMPISession, option *CreateInfraOptions) error {
	pvInstanceClient := instance.NewIBMPIInstanceClient(context.Background(), session, option.PowerVSCloudInstanceID)
	for _, node := range pvInstances {
		pvInstance, err := pvInstanceClient.Get(node.NodeID)

		if err != nil {
			return fmt.Errorf("node: %s, error: %w", node.NodeID, err)
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
				return fmt.Errorf("node: %s, network: %s is Invalid", node.NodeID, nwId)
			}
		}

		log.Log.Info("validated node:", "id", node.NodeID)
	}
	return nil
}

func (managedInfra *ManagedInfra) validateVpc(vpcV1 *vpcv1.VpcV1) error {
	for _, vpc := range managedInfra.Vpc {
		getVpcOpt := vpcv1.GetVPCOptions{ID: &vpc.ID}
		_, _, err := vpcV1.GetVPC(&getVpcOpt)

		if err != nil {
			return err
		}

		getSubnetOpt := vpcv1.GetSubnetOptions{ID: &vpc.SubnetID}
		_, _, err = vpcV1.GetSubnet(&getSubnetOpt)

		if err != nil {
			return fmt.Errorf("subnet: %s, error: %w", vpc.SubnetID, err)
		}

		getLbOpt := vpcv1.GetLoadBalancerOptions{ID: &vpc.LoadBalancerID}
		_, _, err = vpcV1.GetLoadBalancer(&getLbOpt)

		if err != nil {
			return fmt.Errorf("loadBalancer: %s, error: %w", vpc.LoadBalancerID, err)
		}

		log.Log.Info("validated vpc", "id", vpc.ID, "subnet", vpc.SubnetID, "load balancer", vpc.LoadBalancerID)
	}
	return nil
}

func (managedInfra *ManagedInfra) validateCloudConnection(session *ibmpisession.IBMPISession, option *CreateInfraOptions) error {
	pvCloudConClient := instance.NewIBMPICloudConnectionClient(context.Background(), session, option.CloudInstanceID)
	cloudConn, err := pvCloudConClient.Get(managedInfra.CloudConnectionID)

	if err != nil {
		return fmt.Errorf("cloud connection: %s, error: %w", managedInfra.CloudConnectionID, err)
	}

	cloudConnVpc := map[string]bool{}
	vpcEps := cloudConn.Vpc.Vpcs
	for _, vpcEp := range vpcEps {
		crn := strings.Split(*vpcEp.VpcID, ":")
		cloudConnVpc[crn[len(crn)-1]] = true
	}

	for _, vpc := range managedInfra.Vpc {
		if !cloudConnVpc[vpc.ID] {
			return fmt.Errorf("cloud connection: %s, does not have %s vpc connection", managedInfra.CloudConnectionID, vpc.ID)
		}
	}

	cloudConnPvSubnet := map[string]bool{}
	for _, pvSubnet := range cloudConn.Networks {
		cloudConnPvSubnet[*pvSubnet.NetworkID] = true
	}

	for _, pvSubnetId := range managedInfra.PowerVSPrivateSubnet {
		if !cloudConnPvSubnet[pvSubnetId] {
			return fmt.Errorf("cloud connection: %s, does not have %s powervs subnet", managedInfra.CloudConnectionID, pvSubnetId)
		}
	}

	log.Log.Info("validated cloud connection", "id", managedInfra.CloudConnectionID)
	return nil
}
*/
