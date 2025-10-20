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

package controllers

import (
	"context"
	"encoding/base64"
	"slices"
	"time"

	tenantv1 "github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/util/clientcache"
	"github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/util/cloudinit"
	"github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/util/vcdutil"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	types "github.com/vmware/go-vcloud-director/v2/types/v56"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=clouddirectortenantmachines/status,verbs=patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=clouddirectortenantmachines,verbs=get;list;patch;watch

const (
	CloudDirectorTenantMachineFinalizer = "clouddirectortenant.infrastructure.cluster.x-k8s.io/finalizer"
)

var (
	CloudDirectorTenantMachineRequeue = ctrl.Result{RequeueAfter: time.Second * 20}
)

type CloudDirectorTenantMachineReconciler struct {
	client.Client

	ClientCache clientcache.ClientCache
}

func (r *CloudDirectorTenantMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)

	var tenantMachine tenantv1.CloudDirectorTenantMachine
	err := r.Get(ctx, req.NamespacedName, &tenantMachine)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconcile cloud director machine")

	ownerMachine, err := util.GetOwnerMachine(ctx, r.Client, tenantMachine.ObjectMeta)
	if err != nil {
		logger.Error(err, "error getting owner machine")

		return ctrl.Result{}, err
	}

	if ownerMachine == nil {
		logger.Info("ignoring cloud director machine without owner")

		return ctrl.Result{}, nil
	}

	ownerCluster, err := util.GetClusterFromMetadata(ctx, r.Client, ownerMachine.ObjectMeta)
	if err != nil {
		logger.Error(err, "error getting owner cluster")

		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(&tenantMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		err := patchTenantMachine(ctx, patchHelper, &tenantMachine, ownerMachine)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	if !tenantMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &tenantMachine, ownerMachine, ownerCluster)
	}

	if controllerutil.AddFinalizer(&tenantMachine, CloudDirectorTenantMachineFinalizer) {
		return ctrl.Result{}, nil
	}

	if !ownerCluster.Status.InfrastructureReady {
		logger.Info("owner cluster does not have infrastructure ready")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if ownerMachine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("owner machine has no bootstrap data secret name")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if !util.IsControlPlaneMachine(ownerMachine) && !conditions.IsTrue(ownerCluster, clusterv1.ControlPlaneInitializedCondition) {
		logger.Info("ignoring machine until control plane is available")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")

		return ctrl.Result{}, nil
	}

	objectKey := client.ObjectKey{
		Name:      ownerCluster.Spec.InfrastructureRef.Name,
		Namespace: ownerCluster.Namespace,
	}

	var tenantCluster tenantv1.CloudDirectorTenantCluster
	err = r.Get(ctx, objectKey, &tenantCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	if tenantCluster.Spec.IdentityRef == nil {
		logger.Info("ignoring owner cluster without identity reference")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")

		return ctrl.Result{}, nil
	}

	vcdClient, err := r.ClientCache.GetVCDClient(ctx, r.Client, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting client")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

		return ctrl.Result{}, err
	}

	org, err := vcdClient.GetOrgByName(tenantCluster.Spec.Organization)
	if err != nil {
		logger.Error(err, "error getting org")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	vdc, err := org.GetVDCByName(tenantCluster.Spec.VirtualDataCenter, true)
	if err != nil {
		logger.Error(err, "error getting vdc")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	vApp, err := vdc.GetVAppById(tenantCluster.Status.VApp.ID, true)
	if err != nil {
		logger.Error(err, "error getting vapp")

		conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if tenantMachine.Spec.ProviderID == "" {
		logger.Info("no provider id")

		catalog, err := org.GetCatalogByName(tenantMachine.Spec.Catalog, true)
		if vcdutil.IgnoreNotFound(err) != nil {
			logger.Error(err, "error getting catalog", "catalog", tenantMachine.Spec.Catalog)

			conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		if govcd.ContainsNotFound(err) {
			conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CatalogNotFoundReason, clusterv1.ConditionSeverityInfo, "")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		vm, err := vApp.GetVMByName(tenantMachine.Name, true)
		if govcd.ContainsNotFound(err) {
			vAppTemplate, err := catalog.GetVAppTemplateByName(tenantMachine.Spec.Template)
			if vcdutil.IgnoreNotFound(err) != nil {
				conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}

			if govcd.ContainsNotFound(err) {
				conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.TemplateNotFoundReason, clusterv1.ConditionSeverityInfo, "")

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}

			networkConnectionSection := types.NetworkConnectionSection{
				NetworkConnection: []*types.NetworkConnection{
					{
						Network:                 tenantCluster.Spec.Network,
						NetworkConnectionIndex:  0,
						IsConnected:             true,
						IPAddressAllocationMode: types.IPAllocationModePool,
						NetworkAdapterType:      tenantMachine.Spec.NetworkAdapterType,
					},
				},
			}

			task, err := vApp.AddNewVM(tenantMachine.Name, *vAppTemplate, &networkConnectionSection, false)
			if err != nil {
				conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.AddVirtualMachineErrorReason, clusterv1.ConditionSeverityError, err.Error())

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}

			logger.Info("waiting for add new vm task completion", "id", task.Task.ID)

			err = task.WaitTaskCompletion()
			if err != nil {
				logger.Error(err, "error waiting for task completion")
			}

			logger.Info("add new vm task completed", "id", task.Task.ID)

			vm, err = vApp.GetVMByName(tenantMachine.Name, true)
			if err != nil {
				logger.Error(err, "error getting new vm by name")

				conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
		}

		logger.Info("reconciled vapp vm", "id", vm.VM.ID)

		objectKey := client.ObjectKey{
			Name:      *ownerMachine.Spec.Bootstrap.DataSecretName,
			Namespace: ownerMachine.Namespace,
		}

		var secret corev1.Secret
		err = r.Get(ctx, objectKey, &secret)
		if err != nil {
			logger.Error(err, "error getting bootstrap secret")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		value, hasValue := secret.Data["value"]
		if !hasValue {
			logger.Info("bootstrap secret has no value field")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		vAppNetwork, err := vApp.GetVappNetworkByName(tenantCluster.Spec.Network, true)
		if err != nil {
			logger.Error(err, "error getting vapp network")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		networkConnectionSection, err := vm.GetNetworkConnectionSection()
		if err != nil {
			logger.Error(err, "error getting vm network connection section")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		cloudInit, err := cloudinit.ExecuteCloudInitTemplate(vm.VM, vAppNetwork, networkConnectionSection)
		if err != nil {
			logger.Error(err, "error executing cloud init template")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		guestCustomizationSection, err := vm.GetGuestCustomizationSection()
		if err != nil {
			logger.Error(err, "error getting vm guest customization section")

			return CloudDirectorTenantMachineRequeue, nil
		}

		guestCustomizationSection.ComputerName = tenantMachine.Name

		_, err = vm.SetGuestCustomizationSection(guestCustomizationSection)
		if err != nil {
			logger.Error(err, "error setting vm guest customization section")

			return CloudDirectorTenantMachineRequeue, nil
		}

		productSectionList, err := vm.GetProductSectionList()
		if err != nil {
			logger.Error(err, "error getting vm product section list")

			return CloudDirectorTenantMachineRequeue, nil
		}

		hasUserdata := false
		for _, property := range productSectionList.ProductSection.Property {
			if property.Key == "userdata" {
				property.DefaultValue = base64.StdEncoding.EncodeToString(value)

				hasUserdata = true
				break
			}
		}

		if !hasUserdata {
			productSectionList.ProductSection.Property = append(productSectionList.ProductSection.Property, &types.Property{
				Type:         "string",
				Key:          "userdata",
				Label:        "userdata",
				DefaultValue: base64.StdEncoding.EncodeToString(value),
			})
		}

		hasMetadata := false
		for _, property := range productSectionList.ProductSection.Property {
			if property.Key == "metadata" {
				property.DefaultValue = base64.StdEncoding.EncodeToString(cloudInit)

				hasMetadata = true
				break
			}
		}

		if !hasMetadata {
			productSectionList.ProductSection.Property = append(productSectionList.ProductSection.Property, &types.Property{
				Type:         "string",
				Key:          "metadata",
				Label:        "metadata",
				DefaultValue: base64.StdEncoding.EncodeToString(cloudInit),
			})
		}
		_, err = vm.SetProductSectionList(productSectionList)
		if err != nil {
			logger.Error(err, "error setting product section list")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		vmSpecSection := vm.VM.VmSpecSection

		needsUpdate := false

		if tenantMachine.Spec.MemoryResourceMiB != 0 {
			if vmSpecSection.MemoryResourceMb.Configured != tenantMachine.Spec.MemoryResourceMiB {
				vmSpecSection.MemoryResourceMb.Configured = tenantMachine.Spec.MemoryResourceMiB

				needsUpdate = true
			}
		}

		if tenantMachine.Spec.NumCPUs != 0 && *vmSpecSection.NumCpus != tenantMachine.Spec.NumCPUs {
			vmSpecSection.NumCpus = &tenantMachine.Spec.NumCPUs

			needsUpdate = true
		}

		diskSection := diskSectionFromTenantMachine(vmSpecSection.DiskSection, &tenantMachine)
		if diskSection != nil {
			vmSpecSection.DiskSection = diskSection

			needsUpdate = true
		}

		if needsUpdate {
			_, err := vm.UpdateVmSpecSection(vmSpecSection, "dockyards-vcloud-director")
			if err != nil {
				logger.Error(err, "error updating vm spec section")

				conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.UpdateVirtualMachineSpecSectionErrorReason, clusterv1.ConditionSeverityError, err.Error())

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
		}

		task, err := vm.PowerOn()
		if err != nil {
			logger.Error(err, "error powering on vm")

			conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.VirtualMachinePowerOnErrorReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.Info("waiting for vm power on task completion", "id", task.Task.ID)

		err = task.WaitTaskCompletion()
		if err != nil {
			logger.Error(err, "error waiting for power on task")

			conditions.MarkFalse(&tenantMachine, tenantv1.VirtualMachineReadyCondition, tenantv1.VirtualMachinePowerOnErrorReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.Info("vm power on task completed", "id", task.Task.ID)

		tenantMachine.Spec.ProviderID = "vmware-cloud-director://" + vm.VM.ID

		if networkConnectionSection.NetworkConnection != nil {
			tenantMachine.Status.Addresses = []clusterv1.MachineAddress{}
			for _, networkConnection := range networkConnectionSection.NetworkConnection {
				address := clusterv1.MachineAddress{
					Type:    clusterv1.MachineInternalIP,
					Address: networkConnection.IPAddress,
				}

				tenantMachine.Status.Addresses = append(tenantMachine.Status.Addresses, address)
			}
		}
	}

	if util.IsControlPlaneMachine(ownerMachine) {
		logger.Info("owner machine is control plane", "cluster", tenantCluster.Name)

		nsxtFirewallGroup, err := vdc.GetNsxtFirewallGroupById(tenantCluster.Status.IPSet.ID)
		if err != nil {
			logger.Error(err, "errora getting nsxt firewall group")

			conditions.MarkFalse(&tenantMachine, tenantv1.LoadBalancerMemberCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

			return CloudDirectorTenantMachineRequeue, nil
		}

		internalIPAddress := ""
		for _, address := range tenantMachine.Status.Addresses {
			if address.Type == clusterv1.MachineInternalIP {
				internalIPAddress = address.Address
			}
		}

		if internalIPAddress == "" {
			logger.Info("machine has no internal ip address")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		containsIP := slices.ContainsFunc(nsxtFirewallGroup.NsxtFirewallGroup.IpAddresses, func(ipAddress string) bool {
			return ipAddress == internalIPAddress
		})

		if !containsIP {
			logger.Info("machine is not a member of firewall group")

			nsxtFirewallGroup.NsxtFirewallGroup.IpAddresses = append(nsxtFirewallGroup.NsxtFirewallGroup.IpAddresses, internalIPAddress)

			_, err := nsxtFirewallGroup.Update(nsxtFirewallGroup.NsxtFirewallGroup)
			if err != nil {
				logger.Error(err, "error updating firewall group")

				conditions.MarkFalse(&tenantMachine, tenantv1.LoadBalancerMemberCondition, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, err.Error())

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
		}

		conditions.MarkTrue(&tenantMachine, tenantv1.LoadBalancerMemberCondition)
	}

	conditions.MarkTrue(&tenantMachine, tenantv1.VirtualMachineReadyCondition)

	tenantMachine.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *CloudDirectorTenantMachineReconciler) reconcileDelete(ctx context.Context, tenantMachine *tenantv1.CloudDirectorTenantMachine, ownerMachine *clusterv1.Machine, ownerCluster *clusterv1.Cluster) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	if ownerCluster.Spec.InfrastructureRef == nil {
		controllerutil.RemoveFinalizer(tenantMachine, CloudDirectorTenantMachineFinalizer)

		return ctrl.Result{}, nil
	}

	objectKey := client.ObjectKey{
		Name:      ownerCluster.Spec.InfrastructureRef.Name,
		Namespace: ownerCluster.Namespace,
	}

	var tenantCluster tenantv1.CloudDirectorTenantCluster
	err := r.Get(ctx, objectKey, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting cloud director cluster")

		return ctrl.Result{}, err
	}

	vcdClient, err := r.ClientCache.GetVCDClient(ctx, r.Client, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting vcd client")

		return ctrl.Result{}, err
	}

	org, err := vcdClient.GetOrgByName(tenantCluster.Spec.Organization)
	if err != nil {
		logger.Error(err, "error getting org")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	vdc, err := org.GetVDCByName(tenantCluster.Spec.VirtualDataCenter, true)
	if err != nil {
		logger.Error(err, "error getting vdc")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if util.IsControlPlaneMachine(ownerMachine) {
		logger.Info("delete machine is control plane")

		internalIPAdress := ""
		for _, address := range tenantMachine.Status.Addresses {
			if address.Type == clusterv1.MachineInternalIP {
				internalIPAdress = address.Address
			}
		}

		if internalIPAdress != "" {
			logger.Info("removing machine address from firewall group", "address", internalIPAdress)

			nsxtFirewallGroup, err := vdc.GetNsxtFirewallGroupById(tenantCluster.Status.IPSet.ID)
			if err != nil {
				logger.Error(err, "error getting nsxt firewall group")

				return CloudDirectorTenantMachineRequeue, nil
			}

			nsxtFirewallGroup.NsxtFirewallGroup.IpAddresses = slices.DeleteFunc(nsxtFirewallGroup.NsxtFirewallGroup.IpAddresses, func(ipAddress string) bool {
				return ipAddress == internalIPAdress
			})

			_, err = nsxtFirewallGroup.Update(nsxtFirewallGroup.NsxtFirewallGroup)
			if err != nil {
				logger.Error(err, "error updating firewall group")

				return CloudDirectorTenantMachineRequeue, nil
			}
		}
	}

	if tenantCluster.Status.VApp == nil {
		controllerutil.RemoveFinalizer(tenantMachine, CloudDirectorTenantMachineFinalizer)

		return ctrl.Result{}, nil
	}

	vApp, err := vdc.GetVAppById(tenantCluster.Status.VApp.ID, true)
	if err != nil {
		logger.Error(err, "error getting vapp")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	vm, err := vApp.GetVMByName(tenantMachine.Name, true)
	if vcdutil.IgnoreNotFound(err) != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if !govcd.ContainsNotFound(err) {
		err = vm.Delete()
		if err != nil {
			logger.Error(err, "error deleting vm")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	controllerutil.RemoveFinalizer(tenantMachine, CloudDirectorTenantMachineFinalizer)

	return ctrl.Result{}, nil
}

func diskSectionFromTenantMachine(existing *types.DiskSection, tenantMachine *tenantv1.CloudDirectorTenantMachine) *types.DiskSection {
	if tenantMachine.Spec.DiskResourceMiB == 0 && len(tenantMachine.Spec.AdditionalDisks) == 0 {
		return nil
	}

	diskSection := types.DiskSection{
		DiskSettings: []*types.DiskSettings{},
	}

	if tenantMachine.Spec.DiskResourceMiB > 0 {
		diskSettings := existing.DiskSettings[0]

		diskSettings.SizeMb = tenantMachine.Spec.DiskResourceMiB

		diskSection.DiskSettings = append(diskSection.DiskSettings, diskSettings)
	}

	busNumber := existing.DiskSettings[0].BusNumber
	adapterType := existing.DiskSettings[0].AdapterType

	for i, additionalDisk := range tenantMachine.Spec.AdditionalDisks {
		unitNumber := existing.DiskSettings[0].UnitNumber + i + 1
		thinProvisioned := true

		diskSettings := types.DiskSettings{
			AdapterType:     adapterType,
			BusNumber:       busNumber,
			UnitNumber:      unitNumber,
			SizeMb:          additionalDisk.SizeMiB,
			ThinProvisioned: &thinProvisioned,
		}

		diskSection.DiskSettings = append(diskSection.DiskSettings, &diskSettings)
	}

	return &diskSection
}

func (r *CloudDirectorTenantMachineReconciler) clusterToTenantMachines() {
}

func (r *CloudDirectorTenantMachineReconciler) SetupWithManager(manager ctrl.Manager) error {
	scheme := manager.GetScheme()

	_ = tenantv1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	clusterToTenantMachines, err := util.ClusterToTypedObjectsMapper(
		r.Client,
		&tenantv1.CloudDirectorTenantMachineList{},
		scheme,
	)
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(manager).
		For(&tenantv1.CloudDirectorTenantMachine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(
				util.MachineToInfrastructureMapFunc(
					tenantv1.GroupVersion.WithKind(tenantv1.CloudDirectorTenantMachineKind),
				),
			),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToTenantMachines),
		).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}

func patchTenantMachine(ctx context.Context, patchHelper *patch.Helper, tenantMachine *tenantv1.CloudDirectorTenantMachine, machine *clusterv1.Machine, opts ...patch.Option) error {
	summaryConditions := []clusterv1.ConditionType{
		tenantv1.VirtualMachineReadyCondition,
	}

	if util.IsControlPlaneMachine(machine) {
		summaryConditions = append(summaryConditions, tenantv1.LoadBalancerMemberCondition)
	}

	conditions.SetSummary(tenantMachine, conditions.WithConditions(summaryConditions...))

	opts = append(opts, patch.WithOwnedConditions{
		Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			tenantv1.VirtualMachineReadyCondition,
			tenantv1.LoadBalancerMemberCondition,
		},
	})

	return patchHelper.Patch(ctx, tenantMachine, opts...)
}
