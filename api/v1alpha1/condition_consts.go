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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	VirtualServiceReadyCondition clusterv1.ConditionType = "VirtualServiceReady"

	VirtualServiceCreateFailedReason = "VirtualServiceCreateFailed"
)

const (
	PoolReadyCondition clusterv1.ConditionType = "PoolReady"
)

const (
	IPSetReadyCondition clusterv1.ConditionType = "IPSetReady"
)

const (
	VAppReadyCondition clusterv1.ConditionType = "VAppReady"

	VAppCreateFailedReason = "VAppCreateFailed"
)

const (
	ExternalIPAddressReady clusterv1.ConditionType = "ExternalIPAddressReady"

	ExternalIPAddressGetUnusedFailedReason = "ExternalIPAddressGetUnusedFailed"
)

const (
	VirtualMachineReadyCondition clusterv1.ConditionType = "VirtualMachineReady"

	WaitingForClusterInfrastructureReason = "WaitingForClusterInfrastructure"
	WaitingForBootstrapDataReason         = "WaitingForBootstrapData"

	CatalogNotFoundReason  = "CatalogNotFound"
	TemplateNotFoundReason = "TemplateNotFound"

	CloudDirectorErrorReason = "CloudDirectorError"

	AddVirtualMachineErrorReason               = "AddVirtualMachineError"
	UpdateVirtualMachineSpecSectionErrorReason = "UpdateVirtualMachineSpecSectionError"
	VirtualMachinePowerOnErrorReason           = "VirtualMachinePowerOnError"
)

const (
	LoadBalancerMemberCondition clusterv1.ConditionType = "LoadBalancerMember"
)
