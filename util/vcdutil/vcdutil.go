package vcdutil

import (
	"context"
	"net/url"

	tenantv1 "bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

func GetVCDClientFromTenantCluster(ctx context.Context, c client.Client, tenantCluster *tenantv1.CloudDirectorTenantCluster) (*govcd.VCDClient, error) {
	objectKey := client.ObjectKey{
		Name:      tenantCluster.Spec.IdentityRef.Name,
		Namespace: tenantCluster.Namespace,
	}

	var secret corev1.Secret
	err := c.Get(ctx, objectKey, &secret)
	if err != nil {
		return nil, err
	}

	baseURL, has := secret.Data["url"]
	if !has {
		return nil, nil
	}

	token, has := secret.Data["apiToken"]
	if !has {
		return nil, nil
	}

	u, err := url.Parse(string(baseURL) + "/api")
	if err != nil {

		return nil, err
	}

	vcdClient := govcd.NewVCDClient(*u, false)
	err = vcdClient.SetToken(tenantCluster.Spec.Organization, govcd.ApiTokenHeader, string(token))
	if err != nil {

		return nil, err
	}

	return vcdClient, nil
}

func IgnoreNotFound(err error) error {
	if govcd.ContainsNotFound(err) {
		return nil
	}

	return err
}
