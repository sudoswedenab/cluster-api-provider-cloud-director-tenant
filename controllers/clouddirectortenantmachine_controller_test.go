package controllers

import (
	"testing"

	tenantv1 "bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
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
