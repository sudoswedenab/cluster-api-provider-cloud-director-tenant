package cloudinit

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	types "github.com/vmware/go-vcloud-director/v2/types/v56"
)

func TestExecuteCloudInitTemplate(t *testing.T) {
	tt := []struct {
		name                     string
		vm                       types.Vm
		vAppNetwork              types.VAppNetwork
		networkConnectionSection types.NetworkConnectionSection
		expected                 string
	}{
		{
			name: "test single network connection",
			vm: types.Vm{
				ID:   "abc-123",
				Name: "test",
			},
			vAppNetwork: types.VAppNetwork{
				Configuration: &types.NetworkConfiguration{
					IPScopes: &types.IPScopes{
						IPScope: []*types.IPScope{
							{
								Gateway:            "1.2.3.0",
								DNS1:               "4.3.2.1",
								SubnetPrefixLength: "24",
								IPRanges: &types.IPRanges{
									IPRange: []*types.IPRange{
										{
											StartAddress: "1.2.3.4",
											EndAddress:   "4.3.2.1",
										},
									},
								},
							},
						},
					},
				},
			},
			networkConnectionSection: types.NetworkConnectionSection{
				NetworkConnection: []*types.NetworkConnection{
					{
						NetworkConnectionIndex: 0,
						IPAddress:              "1.2.3.4",
						MACAddress:             "aa:bb:cc:dd:ee:ff",
					},
				},
			},
			expected: strings.Join([]string{
				"#cloud-config",
				"local-hostname: test",
				"instance-id: abc-123",
				"network:",
				"  version: 2",
				"  ethernets:",
				"    eth0:",
				"      match:",
				"        macaddress: \"aa:bb:cc:dd:ee:ff\"",
				"      addresses:",
				"      - 1.2.3.4/24",
				"      gateway4: 1.2.3.0",
				"      nameservers:",
				"        addresses:",
				"        - 4.3.2.1",
				"",
			}, "\n"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := ExecuteCloudInitTemplate(&tc.vm, &tc.vAppNetwork, &tc.networkConnectionSection)
			if err != nil {
				t.Fatalf("error executing cloud init template: %s", err)
			}

			if !cmp.Equal(actual, []byte(tc.expected)) {
				t.Errorf("diff: %s", cmp.Diff([]byte(tc.expected), actual))
			}
		})
	}
}
