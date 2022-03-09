package nodepool

import (
	"fmt"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/hypershift-operator/controllers/manifests"
	"github.com/openshift/hypershift/support/releaseinfo"
	v1 "k8s.io/api/core/v1"
	capipowervs "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultCOSRegion = "us-south"
)

// getImageRegion returns the nearest region os IBM COS bucket for the RHCOS images
func getImageRegion(region string) string {
	switch region {
	case "dal", "us-south":
		return "us-south"
	case "eu-de":
		return "eu-de"
	case "lon":
		return "eu-gb"
	case "osa":
		return "jp-osa"
	case "syd":
		return "au-syd"
	case "sao":
		return "br-sao"
	case "tor":
		return "ca-tor"
	case "tok":
		return "jp-tok"
	case "us-east":
		return "us-east"
	default:
		return defaultCOSRegion
	}
}

func ibmPowerVSMachineTemplateSpec(nodePool *hyperv1.NodePool, ami string) *capipowervs.IBMPowerVSMachineTemplateSpec {
	var image *capipowervs.IBMPowerVSResourceReference
	var imageRef *v1.LocalObjectReference
	if nodePool.Spec.Platform.IBMCloudPowerVS.Image != nil {
		image = &capipowervs.IBMPowerVSResourceReference{
			ID:   nodePool.Spec.Platform.IBMCloudPowerVS.Image.ID,
			Name: nodePool.Spec.Platform.IBMCloudPowerVS.Image.Name,
		}
	} else {
		imageRef = &v1.LocalObjectReference{
			Name: ami,
		}
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
				ImageRef:          imageRef,
				Network:           subnet,
				SysType:           "s922",
				ProcType:          "shared",
				Processors:        "0.5",
				Memory:            "32",
			},
		},
	}
}

func ibmPowerVSImageSpec(powervsClusterName, region string, img *releaseinfo.CoreOSPowerVSImage, nodePool *hyperv1.NodePool) *capipowervs.IBMPowerVSImageSpec {
	image := &capipowervs.IBMPowerVSImageSpec{
		ClusterName:       powervsClusterName,
		ServiceInstanceID: nodePool.Spec.Platform.IBMCloudPowerVS.ServiceInstanceID,
		Bucket:            &img.Bucket,
		Object:            &img.Object,
		Region:            &region,
		StorageType:       nodePool.Spec.Platform.IBMCloudPowerVS.StorageType,
		DeletePolicy:      nodePool.Spec.Platform.IBMCloudPowerVS.DeletePolicy,
	}
	return image
}

func getPowerVSImage(pool *hyperv1.NodePool, region string, releaseImage *releaseinfo.ReleaseImage) (*releaseinfo.CoreOSPowerVSImage, string, error) {
	arch, foundArch := releaseImage.StreamMetadata.Architectures["ppc64le"]
	if !foundArch {
		return nil, "", fmt.Errorf("couldn't find OS metadata for architecture %q", "ppc64le")
	}

	COSRegion := getImageRegion(region)

	regionData, hasRegionData := arch.Images.PowerVS.Regions[COSRegion]
	if !hasRegionData {
		return nil, "", fmt.Errorf("couldn't find PowerVS image for region %q", COSRegion)
	}
	return &regionData, COSRegion, nil
}

func ibmPowerVSImageBuilder(hcluster *hyperv1.HostedCluster, nodePool *hyperv1.NodePool, infraID, region string, img *releaseinfo.CoreOSPowerVSImage) (client.Object, func(object client.Object) error, string, error) {
	var image client.Object
	image = &capipowervs.IBMPowerVSImage{}
	imageSpec := ibmPowerVSImageSpec(hcluster.Name, region, img, nodePool)
	mutateImage := func(object client.Object) error {
		o, _ := object.(*capipowervs.IBMPowerVSImage)
		o.Spec = *imageSpec
		if o.Annotations == nil {
			o.Annotations = make(map[string]string)
		}
		o.Annotations[nodePoolAnnotation] = client.ObjectKeyFromObject(nodePool).String()
		o.Annotations[capiv1.ClusterLabelName] = infraID
		return nil
	}
	image.SetNamespace(manifests.HostedControlPlaneNamespace(hcluster.Namespace, hcluster.Name).Name)
	image.SetName(img.Release)
	return image, mutateImage, "", nil
}