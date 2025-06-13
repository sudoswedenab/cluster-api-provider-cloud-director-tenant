package nameutil

import (
	tenantv1 "github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
)

type Use string

const (
	UseControlPlane Use = "controlplane"
)

func ResourceName(tenantCluster *tenantv1.CloudDirectorTenantCluster) string {
	if tenantCluster.Spec.UseUIDForNames {

		return string(tenantCluster.UID)
	}

	return tenantCluster.Name

}

func ResourceNameWithUse(tenantCluster *tenantv1.CloudDirectorTenantCluster, use Use) string {
	if tenantCluster.Spec.UseUIDForNames {
		return string(tenantCluster.UID) + "-" + string(use)
	}

	return tenantCluster.Name + "-" + string(use)
}
