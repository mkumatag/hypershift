package ibmcloud

import (
	"context"
	"fmt"
	k8sutilspointer "k8s.io/utils/pointer"

	configv1 "github.com/openshift/api/config/v1"

	corev1 "k8s.io/api/core/v1"

	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/support/upsert"
	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capiibmv1 "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Release artifacts: https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud/releases/tag/v0.1.0-alpha.3
	// TODO(mkumatag): Move to OpenShift built image
	imageCAPIBM = "gcr.io/k8s-staging-capi-ibmcloud/cluster-api-ibmcloud-controller:v0.1.0-alpha.4"
	//imageCAPIBM = "quay.io/mkumatag/capi-ibm:watch-namespace"
)

type IBMCloud struct {
	Credential string
}

func (p IBMCloud) ReconcileCAPIInfraCR(ctx context.Context, c client.Client, createOrUpdate upsert.CreateOrUpdateFN,
	hcluster *hyperv1.HostedCluster,
	controlPlaneNamespace string,
	apiEndpoint hyperv1.APIEndpoint) (client.Object, error) {
	if hcluster.Spec.Platform.IBMCloud != nil && hcluster.Spec.Platform.IBMCloud.ProviderType == configv1.IBMCloudProviderTypeUPI {
		return nil, nil
	}
	ibmCluster := &capiibmv1.IBMVPCCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlPlaneNamespace,
			Name:      hcluster.Name,
		},
	}

	_, err := createOrUpdate(ctx, c, ibmCluster, func() error {
		ibmCluster.Annotations = map[string]string{
			capiv1.ManagedByAnnotation: "external",
		}

		// Set the values for upper level controller
		ibmCluster.Status.Ready = true
		ibmCluster.Spec.ControlPlaneEndpoint = capiv1.APIEndpoint{
			Host: apiEndpoint.Host,
			Port: apiEndpoint.Port,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// reconciliation strips TypeMeta. We repopulate the static values since they are necessary for
	// downstream reconciliation of the CAPI Cluster resource.
	ibmCluster.TypeMeta = metav1.TypeMeta{
		Kind:       "IBMVPCCluster",
		APIVersion: capiibmv1.GroupVersion.String(),
	}
	return ibmCluster, nil
}

func (p IBMCloud) CAPIProviderDeploymentSpec(hcluster *hyperv1.HostedCluster, tokenMinterImage string) (*appsv1.DeploymentSpec, error) {
	defaultMode := int32(420)
	deploymentSpec := &appsv1.DeploymentSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				TerminationGracePeriodSeconds: k8sutilspointer.Int64Ptr(10),
				Tolerations: []corev1.Toleration{
					{
						Key:    "node-role.kubernetes.io/master",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "capi-webhooks-tls",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								DefaultMode: &defaultMode,
								SecretName:  "capi-webhooks-tls",
							},
						},
					},
					{
						Name: "credentials",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: p.Credential,
							},
						},
					},
					{
						Name: "svc-kubeconfig",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								DefaultMode: &defaultMode,
								SecretName:  "service-network-admin-kubeconfig",
							},
						},
					},
					{
						Name: "token",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{
								Medium: corev1.StorageMediumMemory,
							},
						},
					},
				},
				Containers: []corev1.Container{
					{
						Name:            "manager",
						Image:           imageCAPIBM,
						ImagePullPolicy: corev1.PullAlways,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "credentials",
								MountPath: "/home/.ibmcloud",
							},
							{
								Name:      "capi-webhooks-tls",
								ReadOnly:  true,
								MountPath: "/tmp/k8s-webhook-server/serving-certs",
							},
							{
								Name:      "token",
								MountPath: "/var/run/secrets/openshift/serviceaccount",
							},
						},
						Env: []corev1.EnvVar{
							{
								Name: "MY_NAMESPACE",
								ValueFrom: &corev1.EnvVarSource{
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.namespace",
									},
								},
							},
							{
								Name:  "IBM_CREDENTIALS_FILE",
								Value: "/home/.ibmcloud/ibm-credentials.env",
							},
						},
						Command: []string{"/manager"},
						Args: []string{"--namespace", "$(MY_NAMESPACE)",
							//TODO(mkumatag): Add the log level and stdtoerror post klogr support added.
							"--leader-elect=true",
						},
						// TODO(mkumatag): enable health once fixed in the upstream
						//Ports: []corev1.ContainerPort{
						//	{
						//		Name:          "healthz",
						//		ContainerPort: 9440,
						//		Protocol:      corev1.ProtocolTCP,
						//	},
						//},
						//LivenessProbe: &corev1.Probe{
						//	ProbeHandler: corev1.ProbeHandler{
						//		HTTPGet: &corev1.HTTPGetAction{
						//			Path: "/healthz",
						//			Port: intstr.FromString("healthz"),
						//		},
						//	},
						//},
						//ReadinessProbe: &corev1.Probe{
						//	ProbeHandler: corev1.ProbeHandler{
						//		HTTPGet: &corev1.HTTPGetAction{
						//			Path: "/readyz",
						//			Port: intstr.FromString("healthz"),
						//		},
						//	},
						//},
					},
					{
						Name:            "token-minter",
						Image:           tokenMinterImage,
						ImagePullPolicy: corev1.PullAlways,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "token",
								MountPath: "/var/run/secrets/openshift/serviceaccount",
							},
							{
								Name:      "svc-kubeconfig",
								MountPath: "/etc/kubernetes",
							},
						},
						Command: []string{"/usr/bin/token-minter"},
						Args: []string{
							"-service-account-namespace=kube-system",
							"-service-account-name=capa-controller-manager",
							"-token-audience=openshift",
							"-token-file=/var/run/secrets/openshift/serviceaccount/token",
							"-kubeconfig=/etc/kubernetes/kubeconfig",
						},
					},
				},
			},
		},
	}
	return deploymentSpec, nil
}

func (p IBMCloud) ReconcileCredentials(ctx context.Context, c client.Client, createOrUpdate upsert.CreateOrUpdateFN,
	hcluster *hyperv1.HostedCluster,
	controlPlaneNamespace string) error {
	// Reconcile the platform provider cloud controller credentials secret by resolving
	// the reference from the HostedCluster and syncing the secret in the control
	// plane namespace.
	var src corev1.Secret
	if err := c.Get(ctx, client.ObjectKey{Namespace: hcluster.GetNamespace(), Name: hcluster.Spec.Platform.IBMCloudPowerVS.KubeCloudControllerCreds.Name}, &src); err != nil {
		return fmt.Errorf("failed to get cloud controller provider creds %s: %w", hcluster.Spec.Platform.IBMCloudPowerVS.KubeCloudControllerCreds.Name, err)
	}
	dest := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlPlaneNamespace,
			Name:      src.Name,
		},
	}
	_, err := createOrUpdate(ctx, c, dest, func() error {
		srcData, srcHasData := src.Data["ibm-credentials.env"]
		if !srcHasData {
			return fmt.Errorf("hostedcluster cloud controller provider credentials secret %q must have a credentials key", src.Name)
		}
		dest.Type = corev1.SecretTypeOpaque
		if dest.Data == nil {
			dest.Data = map[string][]byte{}
		}
		dest.Data["credentials"] = srcData
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile cloud controller provider creds: %w", err)
	}

	// Reconcile the platform provider node pool management credentials secret by
	// resolving  the reference from the HostedCluster and syncing the secret in
	// the control plane namespace.
	err = c.Get(ctx, client.ObjectKey{Namespace: hcluster.GetNamespace(), Name: hcluster.Spec.Platform.IBMCloudPowerVS.NodePoolManagementCreds.Name}, &src)
	if err != nil {
		return fmt.Errorf("failed to get node pool provider creds %s: %w", hcluster.Spec.Platform.IBMCloudPowerVS.NodePoolManagementCreds.Name, err)
	}
	dest = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlPlaneNamespace,
			Name:      src.Name,
		},
	}
	_, err = createOrUpdate(ctx, c, dest, func() error {
		srcData, srcHasData := src.Data["ibm-credentials.env"]
		if !srcHasData {
			return fmt.Errorf("node pool provider credentials secret %q is missing credentials key", src.Name)
		}
		dest.Type = corev1.SecretTypeOpaque
		if dest.Data == nil {
			dest.Data = map[string][]byte{}
		}
		dest.Data["credentials"] = srcData
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile node pool provider creds: %w", err)
	}

	// Reconcile the platform provider node pool management credentials secret by
	// resolving  the reference from the HostedCluster and syncing the secret in
	// the control plane namespace.
	err = c.Get(ctx, client.ObjectKey{Namespace: hcluster.GetNamespace(), Name: hcluster.Spec.Platform.IBMCloudPowerVS.ControlPlaneOperatorCreds.Name}, &src)
	if err != nil {
		return fmt.Errorf("failed to get control plane operator provider creds %s: %w", hcluster.Spec.Platform.IBMCloudPowerVS.ControlPlaneOperatorCreds.Name, err)
	}
	dest = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: controlPlaneNamespace,
			Name:      src.Name,
		},
	}
	_, err = createOrUpdate(ctx, c, dest, func() error {
		srcData, srcHasData := src.Data["ibm-credentials.env"]
		if !srcHasData {
			return fmt.Errorf("control plane operator provider credentials secret %q is missing credentials key", src.Name)
		}
		dest.Type = corev1.SecretTypeOpaque
		if dest.Data == nil {
			dest.Data = map[string][]byte{}
		}
		dest.Data["credentials"] = srcData
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile control plane operator provider creds: %w", err)
	}
	return nil
}

func (IBMCloud) ReconcileSecretEncryption(ctx context.Context, c client.Client, createOrUpdate upsert.CreateOrUpdateFN,
	hcluster *hyperv1.HostedCluster,
	controlPlaneNamespace string) error {
	if hcluster.Spec.SecretEncryption.KMS.IBMCloud == nil {
		return fmt.Errorf("ibm kms metadata nil")
	}
	if hcluster.Spec.SecretEncryption.KMS.IBMCloud.Auth.Type == hyperv1.IBMCloudKMSUnmanagedAuth {
		if hcluster.Spec.SecretEncryption.KMS.IBMCloud.Auth.Unmanaged == nil || len(hcluster.Spec.SecretEncryption.KMS.IBMCloud.Auth.Unmanaged.Credentials.Name) == 0 {
			return fmt.Errorf("ibm unmanaged auth credential nil")
		}
		var src corev1.Secret
		if err := c.Get(ctx, client.ObjectKey{Namespace: hcluster.GetNamespace(), Name: hcluster.Spec.SecretEncryption.KMS.IBMCloud.Auth.Unmanaged.Credentials.Name}, &src); err != nil {
			return fmt.Errorf("failed to get ibmcloud kms credentials %s: %w", hcluster.Spec.SecretEncryption.KMS.IBMCloud.Auth.Unmanaged.Credentials.Name, err)
		}
		if _, ok := src.Data[hyperv1.IBMCloudIAMAPIKeySecretKey]; !ok {
			return fmt.Errorf("no ibmcloud iam apikey field %s specified in auth secret", hyperv1.IBMCloudIAMAPIKeySecretKey)
		}
		hostedControlPlaneIBMCloudKMSAuthSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: controlPlaneNamespace,
				Name:      src.Name,
			},
		}
		_, err := createOrUpdate(ctx, c, hostedControlPlaneIBMCloudKMSAuthSecret, func() error {
			if hostedControlPlaneIBMCloudKMSAuthSecret.Data == nil {
				hostedControlPlaneIBMCloudKMSAuthSecret.Data = map[string][]byte{}
			}
			hostedControlPlaneIBMCloudKMSAuthSecret.Data[hyperv1.IBMCloudIAMAPIKeySecretKey] = src.Data[hyperv1.IBMCloudIAMAPIKeySecretKey]
			hostedControlPlaneIBMCloudKMSAuthSecret.Type = corev1.SecretTypeOpaque
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed reconciling aescbc backup key: %w", err)
		}
	}
	return nil
}

func (IBMCloud) CAPIProviderPolicyRules() []rbacv1.PolicyRule {
	return nil
}
