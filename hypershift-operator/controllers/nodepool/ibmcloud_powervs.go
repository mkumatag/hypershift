package nodepool

import (
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	capipowervs "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta1"
)

func ibmPowerVSMachineTemplateSpec(nodePool *hyperv1.NodePool) *capipowervs.IBMPowerVSMachineTemplateSpec {
	image := capipowervs.IBMPowerVSResourceReference{}
	if nodePool.Spec.Platform.IBMCloudPowerVS.Image != nil {
		image.ID = nodePool.Spec.Platform.IBMCloudPowerVS.Image.ID
		image.Name = nodePool.Spec.Platform.IBMCloudPowerVS.Image.Name
	}
	subnet := capipowervs.IBMPowerVSResourceReference{}
	if nodePool.Spec.Platform.IBMCloudPowerVS.Subnet != nil {
		subnet.ID = nodePool.Spec.Platform.IBMCloudPowerVS.Subnet.ID
		subnet.Name = nodePool.Spec.Platform.IBMCloudPowerVS.Subnet.Name
	}
	return &capipowervs.IBMPowerVSMachineTemplateSpec{
		Template: capipowervs.IBMPowerVSMachineTemplateResource{
			Spec: capipowervs.IBMPowerVSMachineSpec{
				ServiceInstanceID: nodePool.Spec.Platform.IBMCloudPowerVS.ServiceInstanceID,
				Image:             image,
				Network:           subnet,
				SysType:           "s922",
				ProcType:          "shared",
				Processors:        "0.25",
				Memory:            "8",
			},
		},
	}
}
