package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	CloudDirectorTenantClusterKind = "CloudDirectorTenantCluster"
)

type CloudDirectorTenantClusterSpec struct {
	// +optional
	ControlPlaneEndpoint v1beta1.APIEndpoint `json:"controlPlaneEndpoint"`

	Org         string `json:"org"`
	VDC         string `json:"vdc"`
	EdgeGateway string `json:"edgeGateway"`
	Network     string `json:"network"`

	IdentityRef *CloudDirectorTenantIdentityRef `json:"identityRef,omitempty"`
}

type CloudDirectorTenantClusterStatus struct {
	// +kubebuilder:default=false
	Ready                        bool   `json:"ready"`
	LoadBalancerVirtualServiceID string `json:"loadBalancerVirtualServiceID,omitempty"`
	VAppID                       string `json:"vAppID,omitempty"`
	FirewallGroupID              string `json:"firewallGroupID,omitempty"`
	AlbPoolID                    string `json:"albPoolID,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type CloudDirectorTenantCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudDirectorTenantClusterSpec   `json:"spec,omitempty"`
	Status CloudDirectorTenantClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type CloudDirectorTenantClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []CloudDirectorTenantCluster `json:"items,omitempty"`
}

func init() {
	SchemeBuilder.Register(&CloudDirectorTenantCluster{}, &CloudDirectorTenantClusterList{})
}
