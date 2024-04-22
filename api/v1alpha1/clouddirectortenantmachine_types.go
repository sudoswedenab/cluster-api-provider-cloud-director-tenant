package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type CloudDirectorTenantMachineSpec struct {
	ProviderID string `json:"providerID,omitempty"`

	Catalog  string `json:"catalog"`
	Template string `json:"template"`

	NetworkAdapterType string           `json:"networkAdapterType,omitempty"`
	MemoryResourceMiB  int64            `json:"memoryResourceMiB,omitempty"`
	NumCPUs            int              `json:"numCPUs,omitempty"`
	DiskResourceMiB    int64            `json:"diskResourceMiB,omitempty"`
	AdditionalDisks    []AdditionalDisk `json:"additionalDisks,omitempty"`
}

type CloudDirectorTenantMachineStatus struct {
	// +optional
	Ready bool `json:"ready"`

	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
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

func (m *CloudDirectorTenantMachine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

func (m *CloudDirectorTenantMachine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&CloudDirectorTenantMachine{}, &CloudDirectorTenantMachineList{})
}
