// Copyright 2025 Sudo Sweden AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
