package controllers

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/url"
	"slices"
	"time"

	"bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/util/cloudinit"
	"bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/util/vcdutil"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	types "github.com/vmware/go-vcloud-director/v2/types/v56"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CloudDirectorTenantMachineFinalizer = "clouddirectortenant.infrastructure.cluster.x-k8s.io/finalizer"
)

var (
	CloudDirectorTenantMachineRequeue = ctrl.Result{RequeueAfter: time.Second * 20}
)

type CloudDirectorTenantMachineReconciler struct {
	client.Client
	Logger *slog.Logger
}

func (r *CloudDirectorTenantMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var cloudDirectorTenantMachine v1alpha1.CloudDirectorTenantMachine
	err := r.Get(ctx, req.NamespacedName, &cloudDirectorTenantMachine)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconcile cloud director machine")

	ownerMachine, err := util.GetOwnerMachine(ctx, r.Client, cloudDirectorTenantMachine.ObjectMeta)
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

	if !cloudDirectorTenantMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &cloudDirectorTenantMachine, ownerMachine, ownerCluster)
	}

	if !controllerutil.ContainsFinalizer(&cloudDirectorTenantMachine, CloudDirectorTenantMachineFinalizer) {
		patch := client.MergeFrom(cloudDirectorTenantMachine.DeepCopy())

		if controllerutil.AddFinalizer(&cloudDirectorTenantMachine, CloudDirectorTenantMachineFinalizer) {
			err := r.Patch(ctx, &cloudDirectorTenantMachine, patch)
			if err != nil {
				logger.Error(err, "error patching finalizer")

				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}
	}

	if !ownerCluster.Status.InfrastructureReady {
		logger.Info("owner cluster does not have infrastructure ready")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if ownerMachine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("owner machine has no bootstrap data secret name")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	objectKey := client.ObjectKey{
		Name:      ownerCluster.Spec.InfrastructureRef.Name,
		Namespace: ownerCluster.Namespace,
	}

	var tenantCluster v1alpha1.CloudDirectorTenantCluster
	err = r.Get(ctx, objectKey, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting cloud director cluster")

		return ctrl.Result{}, err
	}

	if tenantCluster.Spec.IdentityRef == nil {
		logger.Info("ignoring owner cluster without identity reference")

		return ctrl.Result{}, nil
	}

	vcdClient, err := vcdutil.GetVCDClientFromTenantCluster(ctx, r.Client, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting client")

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

	vApp, err := vdc.GetVAppById(tenantCluster.Status.VApp.ID, true)
	if err != nil {
		logger.Error(err, "error getting vapp")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if cloudDirectorTenantMachine.Spec.ProviderID == "" {
		logger.Info("no provider id")

		catalog, err := org.GetCatalogByName(cloudDirectorTenantMachine.Spec.Catalog, true)
		if err != nil {
			logger.Error(err, "error getting catalog", "catalog", cloudDirectorTenantMachine.Spec.Catalog)

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		vm, err := vApp.GetVMByName(cloudDirectorTenantMachine.Name, true)
		if govcd.ContainsNotFound(err) {
			vAppTemplate, err := catalog.GetVAppTemplateByName(cloudDirectorTenantMachine.Spec.Template)
			if err != nil {
				logger.Error(err, "error getting vapp template")

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}

			networkConnectionSection := types.NetworkConnectionSection{
				NetworkConnection: []*types.NetworkConnection{
					{
						Network:                 tenantCluster.Spec.Network,
						NetworkConnectionIndex:  0,
						IsConnected:             true,
						IPAddressAllocationMode: types.IPAllocationModePool,
						NetworkAdapterType:      cloudDirectorTenantMachine.Spec.NetworkAdapterType,
					},
				},
			}

			task, err := vApp.AddNewVM(cloudDirectorTenantMachine.Name, *vAppTemplate, &networkConnectionSection, false)
			if err != nil {
				logger.Error(err, "error adding new vm")

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}

			logger.Info("waiting for add new vm task completion", "id", task.Task.ID)

			err = task.WaitTaskCompletion()
			if err != nil {
				logger.Error(err, "error waiting for task completion")
			}

			logger.Info("add new vm task completed", "id", task.Task.ID)

			vm, err = vApp.GetVMByName(cloudDirectorTenantMachine.Name, true)
			if err != nil {
				logger.Error(err, "error getting new vm by name")

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

		// fmt.Printf("vappnetwrk: %#v\n,configuration: %#v\nipscopes: %#v", vAppNetwork, vAppNetwork.Configuration, vAppNetwork.Configuration.IPScopes)

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

		guestCustomizationSection.ComputerName = cloudDirectorTenantMachine.Name

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

		vmSpecSection.DiskSection = nil
		needsUpdate := false

		if cloudDirectorTenantMachine.Spec.MemoryResourceMiB != 0 {
			if vmSpecSection.MemoryResourceMb.Configured != cloudDirectorTenantMachine.Spec.MemoryResourceMiB {
				vmSpecSection.MemoryResourceMb.Configured = cloudDirectorTenantMachine.Spec.MemoryResourceMiB

				needsUpdate = true
			}
		}

		if cloudDirectorTenantMachine.Spec.NumCPUs != 0 && *vmSpecSection.NumCpus != cloudDirectorTenantMachine.Spec.NumCPUs {
			vmSpecSection.NumCpus = &cloudDirectorTenantMachine.Spec.NumCPUs

			needsUpdate = true
		}

		if needsUpdate {
			_, err := vm.UpdateVmSpecSection(vmSpecSection, "dockyards-vcloud-director")
			if err != nil {
				logger.Error(err, "error updating vm spec section")

				return ctrl.Result{}, err
			}
		}

		task, err := vm.PowerOn()
		if err != nil {
			logger.Error(err, "error powering on vm")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.Info("waiting for vm power on task completion", "id", task.Task.ID)

		err = task.WaitTaskCompletion()
		if err != nil {
			logger.Error(err, "error waiting for power on task")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.Info("vm power on task completed", "id", task.Task.ID)

		patch := client.MergeFrom(cloudDirectorTenantMachine.DeepCopy())

		cloudDirectorTenantMachine.Spec.ProviderID = "vmware-cloud-director://" + vm.VM.ID

		err = r.Patch(ctx, &cloudDirectorTenantMachine, patch)
		if err != nil {
			logger.Error(err, "error patching machine")

			return ctrl.Result{}, err

		}

		if networkConnectionSection.NetworkConnection != nil {
			cloudDirectorTenantMachine.Status.Addresses = []clusterv1.MachineAddress{}
			for _, networkConnection := range networkConnectionSection.NetworkConnection {
				address := clusterv1.MachineAddress{
					Type:    clusterv1.MachineInternalIP,
					Address: networkConnection.IPAddress,
				}

				cloudDirectorTenantMachine.Status.Addresses = append(cloudDirectorTenantMachine.Status.Addresses, address)
			}
		}

		cloudDirectorTenantMachine.Status.Ready = true

		err = r.Status().Patch(ctx, &cloudDirectorTenantMachine, patch)
		if err != nil {
			logger.Error(err, "error patching machine status")

			return ctrl.Result{}, err
		}
	}

	if util.IsControlPlaneMachine(ownerMachine) {
		logger.Info("owner machine is control plane", "cluster", tenantCluster.Name)

		nsxtFirewallGroup, err := vdc.GetNsxtFirewallGroupById(tenantCluster.Status.IPSet.ID)
		if err != nil {
			logger.Error(err, "errora getting nsxt firewall group")

			return CloudDirectorTenantMachineRequeue, nil
		}

		internalIPAddress := ""
		for _, address := range cloudDirectorTenantMachine.Status.Addresses {
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

				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *CloudDirectorTenantMachineReconciler) reconcileDelete(ctx context.Context, cloudDirectorTenantMachine *v1alpha1.CloudDirectorTenantMachine, ownerMachine *clusterv1.Machine, ownerCluster *clusterv1.Cluster) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	objectKey := client.ObjectKey{
		Name:      ownerCluster.Spec.InfrastructureRef.Name,
		Namespace: ownerCluster.Namespace,
	}

	var tenantCluster v1alpha1.CloudDirectorTenantCluster
	err := r.Get(ctx, objectKey, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting cloud director cluster")

		return ctrl.Result{}, err
	}

	objectKey = client.ObjectKey{
		Name:      tenantCluster.Spec.IdentityRef.Name,
		Namespace: tenantCluster.Namespace,
	}

	var secret corev1.Secret
	err = r.Get(ctx, objectKey, &secret)
	if err != nil {
		logger.Error(err, "error getting identity secret")

		return ctrl.Result{}, err
	}

	vcdURL, has := secret.Data["vcdEndpoint"]
	if !has {
		logger.Info("ignoring cluster without vcd endpoint")

		return ctrl.Result{}, nil
	}

	token, has := secret.Data["apiToken"]
	if !has {
		logger.Info("ignoring cluster without api token")

		return ctrl.Result{}, nil
	}

	u, err := url.Parse(string(vcdURL))
	if err != nil {
		logger.Error(err, "error parsing vcd endpoint url")

		return ctrl.Result{}, err
	}

	vcdClient := govcd.NewVCDClient(*u, false)
	err = vcdClient.SetToken(tenantCluster.Spec.Organization, govcd.ApiTokenHeader, string(token))
	if err != nil {
		logger.Error(err, "error setting token")

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
		for _, address := range cloudDirectorTenantMachine.Status.Addresses {
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

	vApp, err := vdc.GetVAppById(tenantCluster.Status.VApp.ID, true)
	if err != nil {
		logger.Error(err, "error getting vapp")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	vm, err := vApp.GetVMByName(cloudDirectorTenantMachine.Name, true)
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

	patch := client.MergeFrom(cloudDirectorTenantMachine.DeepCopy())

	if controllerutil.RemoveFinalizer(cloudDirectorTenantMachine, CloudDirectorTenantMachineFinalizer) {
		err := r.Patch(ctx, cloudDirectorTenantMachine, patch)
		if err != nil {
			logger.Error(err, "error patching finalizer")

			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *CloudDirectorTenantMachineReconciler) SetupWithManager(manager ctrl.Manager) error {
	scheme := manager.GetScheme()

	_ = v1alpha1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(manager).For(&v1alpha1.CloudDirectorTenantMachine{}).Complete(r)
	if err != nil {
		return err
	}

	return nil
}
