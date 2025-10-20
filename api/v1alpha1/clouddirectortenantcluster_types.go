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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	CloudDirectorTenantClusterKind = "CloudDirectorTenantCluster"
)

type CloudDirectorTenantClusterSpec struct {
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint"`

	Organization       string `json:"organization"`
	VirtualDataCenter  string `json:"virtualDataCenter"`
	EdgeGateway        string `json:"edgeGateway"`
	Network            string `json:"network"`
	ServiceEngineGroup string `json:"serviceEngineGroup"`

	IdentityRef *CloudDirectorTenantIdentityRef `json:"identityRef,omitempty"`

	UseUIDForNames bool `json:"useUIDForNames,omitempty"`
}

type CloudDirectorReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CloudDirectorTenantClusterStatus struct {
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	VirtualService *CloudDirectorReference `json:"virtualService,omitempty"`
	VApp           *CloudDirectorReference `json:"vApp,omitempty"`
	IPSet          *CloudDirectorReference `json:"ipSet,omitempty"`
	Pool           *CloudDirectorReference `json:"pool,omitempty"`

	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
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

func (c *CloudDirectorTenantCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *CloudDirectorTenantCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&CloudDirectorTenantCluster{}, &CloudDirectorTenantClusterList{})
}
