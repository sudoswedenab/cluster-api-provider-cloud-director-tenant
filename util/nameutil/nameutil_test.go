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

package nameutil_test

import (
	"testing"

	tenantv1 "github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/util/nameutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResourceName(t *testing.T) {
	tt := []struct {
		name     string
		cluster  tenantv1.CloudDirectorTenantCluster
		expected string
	}{
		{
			name: "test cluster",
			cluster: tenantv1.CloudDirectorTenantCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "testing",
					UID:       "cf22dd54-e459-4c74-8352-93476f6afad2",
				},
			},
			expected: "test",
		},
		{
			name: "test cluster with uid",
			cluster: tenantv1.CloudDirectorTenantCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "testing",
					UID:       "d41436d2-4cd8-458e-a4d9-df781b2363aa",
				},
				Spec: tenantv1.CloudDirectorTenantClusterSpec{
					UseUIDForNames: true,
				},
			},
			expected: "d41436d2-4cd8-458e-a4d9-df781b2363aa",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			actual := nameutil.ResourceName(&tc.cluster)
			if actual != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, actual)
			}
		})
	}
}

func TestResourceNameWithUse(t *testing.T) {
	tt := []struct {
		name     string
		cluster  tenantv1.CloudDirectorTenantCluster
		use      nameutil.Use
		expected string
	}{
		{
			name: "test cluster with controlplane use",
			cluster: tenantv1.CloudDirectorTenantCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "testing",
					UID:       "9e635b3a-2230-40f7-b725-1b3ce7ac8390",
				},
			},
			use:      nameutil.UseControlPlane,
			expected: "test-controlplane",
		},
		{
			name: "test cluster with uid and controlplane use",
			cluster: tenantv1.CloudDirectorTenantCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "testing",
					UID:       "6854816c-7b1f-433e-ab5c-43e110d2d046",
				},
				Spec: tenantv1.CloudDirectorTenantClusterSpec{
					UseUIDForNames: true,
				},
			},
			use:      nameutil.UseControlPlane,
			expected: "6854816c-7b1f-433e-ab5c-43e110d2d046-controlplane",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			actual := nameutil.ResourceNameWithUse(&tc.cluster, tc.use)
			if actual != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, actual)
			}
		})
	}
}
