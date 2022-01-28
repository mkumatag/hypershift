package fixtures

import (
	corev1 "k8s.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Resources = &ExampleIBMCloudPowerVSResources{}

type ExampleIBMCloudPowerVSOptions struct {
	ApiKey string
}

type ExampleIBMCloudPowerVSResources struct {
	KubeCloudControllerIBMCloudPowerVSCreds  *corev1.Secret
	NodePoolManagementIBMCloudPowerVSCreds   *corev1.Secret
	ControlPlaneOperatorIBMCloudPowerVSCreds *corev1.Secret
}

func (o *ExampleIBMCloudPowerVSResources) AsObjects() []crclient.Object {
	var objects []crclient.Object
	if o.KubeCloudControllerIBMCloudPowerVSCreds != nil {
		objects = append(objects, o.KubeCloudControllerIBMCloudPowerVSCreds)
	}
	if o.NodePoolManagementIBMCloudPowerVSCreds != nil {
		objects = append(objects, o.NodePoolManagementIBMCloudPowerVSCreds)
	}
	if o.ControlPlaneOperatorIBMCloudPowerVSCreds != nil {
		objects = append(objects, o.ControlPlaneOperatorIBMCloudPowerVSCreds)
	}
	return objects
}

type ExampleIBMCloudVPCOptions struct {
}
