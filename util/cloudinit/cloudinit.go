package cloudinit

import (
	"bytes"
	"net/netip"
	"text/template"

	types "github.com/vmware/go-vcloud-director/v2/types/v56"
)

// https://cloudinit.readthedocs.io/en/23.3.3/reference/network-config-format-v2.html
const cloudInitTemplate = `#cloud-config
local-hostname: {{ .VM.Name}}
instance-id: {{ .VM.ID}}
network:
  version: 2
  ethernets:
    {{- range .Ethernets }}
    eth{{ .NetworkConnection.NetworkConnectionIndex }}:
      match:
        macaddress: "{{ .NetworkConnection.MACAddress }}"
      addresses:
      - {{ .NetworkConnection.IPAddress }}/{{ .IPScope.SubnetPrefixLength }}
      gateway4: {{ .IPScope.Gateway }}
      nameservers:
        addresses:
        - {{ .IPScope.DNS1 }}
      {{- end }}
`

type Ethernet struct {
	IPScope           *types.IPScope
	NetworkConnection *types.NetworkConnection
}

func ExecuteCloudInitTemplate(vm *types.Vm, vAppNetwork *types.VAppNetwork, networkConnectionSection *types.NetworkConnectionSection) ([]byte, error) {
	t, err := template.New("cloud-init").Parse(cloudInitTemplate)
	if err != nil {
		return []byte{}, err
	}

	ethernets := make([]Ethernet, len(networkConnectionSection.NetworkConnection))

	for i, networkConnection := range networkConnectionSection.NetworkConnection {
		ethernet := Ethernet{
			NetworkConnection: networkConnectionSection.NetworkConnection[i],
		}

		ipAddress, err := netip.ParseAddr(networkConnection.IPAddress)
		if err != nil {
			return []byte{}, err
		}

		for j, ipScope := range vAppNetwork.Configuration.IPScopes.IPScope {
			if ipScope.IPRanges == nil {
				// fmt.Println("scope has no ip ranges", ipScope.Netmask)

				continue
			}

			for _, ipRange := range ipScope.IPRanges.IPRange {
				// fmt.Println("start", ipRange.StartAddress, "length", ipScope.SubnetPrefixLength)

				startAddress, err := netip.ParseAddr(ipRange.StartAddress)
				if err != nil {
					return []byte{}, err
				}

				endAddress, err := netip.ParseAddr(ipRange.EndAddress)
				if err != nil {
					return []byte{}, err
				}

				if ipAddress.Compare(startAddress) == -1 || ipAddress.Compare(endAddress) == 1 {
					continue
				}

				ethernet.IPScope = vAppNetwork.Configuration.IPScopes.IPScope[j]
			}
		}

		ethernets[i] = ethernet
	}

	data := struct {
		VM        *types.Vm
		Ethernets []Ethernet
	}{
		VM:        vm,
		Ethernets: ethernets,
	}

	var buffer bytes.Buffer
	err = t.Execute(&buffer, data)
	if err != nil {
		return []byte{}, err
	}

	return buffer.Bytes(), nil
}
