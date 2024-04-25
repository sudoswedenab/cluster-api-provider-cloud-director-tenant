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
