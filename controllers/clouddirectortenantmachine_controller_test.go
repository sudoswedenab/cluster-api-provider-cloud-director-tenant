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

package controllers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	tenantv1 "github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	types "github.com/vmware/go-vcloud-director/v2/types/v56"
)

func TestDiskSectionFromMachine(t *testing.T) {
	tt := []struct {
		name     string
		existing types.DiskSection
		machine  tenantv1.CloudDirectorTenantMachine
		expected *types.DiskSection
	}{
		{
			name: "test empty",
			existing: types.DiskSection{
				DiskSettings: []*types.DiskSettings{
					{
						SizeMb: 123,
					},
				},
			},
			machine: tenantv1.CloudDirectorTenantMachine{
				Spec: tenantv1.CloudDirectorTenantMachineSpec{},
			},
		},
		{
			name: "test disk resource",
			existing: types.DiskSection{
				DiskSettings: []*types.DiskSettings{
					{
						AdapterType: "hello",
						BusNumber:   0,
						UnitNumber:  0,
						SizeMb:      123,
					},
				},
			},
			machine: tenantv1.CloudDirectorTenantMachine{
				Spec: tenantv1.CloudDirectorTenantMachineSpec{
					DiskResourceMiB: 123456,
				},
			},
			expected: &types.DiskSection{
				DiskSettings: []*types.DiskSettings{
					{
						BusNumber:  0,
						UnitNumber: 0,
						SizeMb:     123456,
					},
				},
			},
		},
		{
			name: "test additional disk",
			existing: types.DiskSection{
				DiskSettings: []*types.DiskSettings{
					{
						AdapterType: "existing",
						BusNumber:   0,
						UnitNumber:  0,
						SizeMb:      123,
					},
				},
			},
			machine: tenantv1.CloudDirectorTenantMachine{
				Spec: tenantv1.CloudDirectorTenantMachineSpec{
					AdditionalDisks: []tenantv1.AdditionalDisk{
						{
							Name:    "test",
							SizeMiB: 123456,
						},
					},
				},
			},
			expected: &types.DiskSection{
				DiskSettings: []*types.DiskSettings{
					{
						AdapterType: "existing",
						BusNumber:   0,
						UnitNumber:  1,
						SizeMb:      123456,
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			actual := diskSectionFromTenantMachine(&tc.existing, &tc.machine)

			if !cmp.Equal(actual, tc.expected) {
				t.Errorf("diff: %s", cmp.Diff(tc.expected, actual))
			}
		})
	}
}
