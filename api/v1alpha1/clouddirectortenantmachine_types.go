package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

type CloudDirectorTenantMachineSpec struct {
	ProviderID string `json:"providerID,omitempty"`

	Catalog  string `json:"catalog"`
	Template string `json:"template"`

	NetworkAdapterType string `json:"networkAdapterType,omitempty"`
	MemoryResourceMiB  int64  `json:"memoryResourceMiB,omitempty"`
	NumCPUs            int    `json:"numCPUs,omitempty"`
}

type CloudDirectorTenantMachineStatus struct {
	// +optional
	Ready bool `json:"ready"`

	Addresses []v1beta1.MachineAddress `json:"addresses,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type CloudDirectorTenantMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudDirectorTenantMachineSpec   `json:"spec,omitempty"`
	Status CloudDirectorTenantMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type CloudDirectorTenantMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []CloudDirectorTenantMachine `json:"items,omitempty"`
}

func init() {
	SchemeBuilder.Register(&CloudDirectorTenantMachine{}, &CloudDirectorTenantMachineList{})
}
