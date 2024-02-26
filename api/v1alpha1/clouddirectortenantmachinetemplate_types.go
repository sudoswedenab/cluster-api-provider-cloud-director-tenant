package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	CloudDirectorTenantMachineTemplateKind = "CloudDirectorTenantMachineTemplate"
)

type CloudDirectorTenantMachineTemplateResource struct {
	ObjectMeta v1beta1.ObjectMeta             `json:"metadata,omitempty"`
	Spec       CloudDirectorTenantMachineSpec `json:"spec"`
}

type CloudDirectorTenantMachineTemplateSpec struct {
	Template CloudDirectorTenantMachineTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
type CloudDirectorTenantMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CloudDirectorTenantMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type CloudDirectorTenantMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []CloudDirectorTenantMachineTemplate `json:"items,omitempty"`
}

func init() {
	SchemeBuilder.Register(&CloudDirectorTenantMachineTemplate{}, &CloudDirectorTenantMachineTemplateList{})
}
