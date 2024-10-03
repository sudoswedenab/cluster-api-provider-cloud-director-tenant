package controllers

import (
	"context"
	"log/slog"
	"net/netip"
	"net/url"
	"time"

	tenantv1 "bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"bitbucket.org/sudosweden/cluster-api-provider-cloud-director-tenant/util/vcdutil"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	typesv56 "github.com/vmware/go-vcloud-director/v2/types/v56"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=clouddirectortenantclusters,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=clouddirectortenantclusters/status,verbs=patch

const (
	CloudDirectorTenantClusterFinalizer = "cloud-director-tenant.infrastructure.cluster.x-k8s.io/finalizer"
)

type CloudDirectorTenantClusterReconciler struct {
	client.Client
	Logger *slog.Logger
}

func (r *CloudDirectorTenantClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := ctrl.LoggerFrom(ctx)

	var tenantCluster tenantv1.CloudDirectorTenantCluster
	err := r.Get(ctx, req.NamespacedName, &tenantCluster)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("reconcile cluster")

	patchHelper, err := patch.NewHelper(&tenantCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		err := patchTenantCluster(ctx, patchHelper, &tenantCluster)
		if err != nil {
			result = ctrl.Result{}
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()

	ownerCluster, err := util.GetOwnerCluster(ctx, r.Client, tenantCluster.ObjectMeta)
	if err != nil {
		logger.Error(err, "error getting owner cluster")

		return ctrl.Result{}, err
	}

	if !tenantCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &tenantCluster, ownerCluster)
	}

	if controllerutil.AddFinalizer(&tenantCluster, CloudDirectorTenantClusterFinalizer) {
		return ctrl.Result{}, nil
	}

	if tenantCluster.Spec.IdentityRef == nil {
		logger.Info("ignoring cluster without identity reference")

		return ctrl.Result{}, nil
	}

	vcdClient, err := vcdutil.GetVCDClientFromTenantCluster(ctx, r.Client, &tenantCluster)
	if err != nil {
		logger.Error(err, "error getting client")

		conditions.MarkFalse(&tenantCluster, tenantv1.ExternalIPAddressReady, tenantv1.CloudDirectorErrorReason, clusterv1.ConditionSeverityError, "%s", err)

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	org, err := vcdClient.GetOrgByName(tenantCluster.Spec.Organization)
	if err != nil {
		logger.Error(err, "error getting org from vcloud")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger.Info("got org", "id", org.Org.ID)

	vdc, err := org.GetVDCByName(tenantCluster.Spec.VirtualDataCenter, false)
	if err != nil {
		logger.Error(err, "error getting vdc")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	nsxtEdgeGateway, err := org.GetNsxtEdgeGatewayByNameAndOwnerId(tenantCluster.Spec.EdgeGateway, vdc.Vdc.ID)
	if err != nil {
		logger.Error(err, "error getting edge gateway")

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if tenantCluster.Spec.ControlPlaneEndpoint.Host == "" {
		addrs, err := nsxtEdgeGateway.GetUnusedExternalIPAddresses(1, netip.Prefix{}, true)
		if err != nil {
			logger.Error(err, "error getting unused ip addresses")

			conditions.MarkFalse(&tenantCluster, tenantv1.ExternalIPAddressReady, tenantv1.ExternalIPAddressGetUnusedFailedReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		tenantCluster.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
			Host: addrs[0].String(),
			Port: int32(6443),
		}

		return ctrl.Result{}, nil
	}

	conditions.MarkTrue(&tenantCluster, tenantv1.ExternalIPAddressReady)

	if ownerCluster == nil {
		logger.Info("ignoring cloud director cluster without cluster owner")

		return ctrl.Result{}, nil
	}

	if ownerCluster.Spec.ControlPlaneRef == nil {
		logger.Info("ignoring cluster without control plane ref")

		return ctrl.Result{}, nil
	}

	nsxtFirewallGroup, err := vdc.GetNsxtFirewallGroupByName(ownerCluster.Spec.ControlPlaneRef.Name, typesv56.FirewallGroupTypeIpSet)
	if govcd.ContainsNotFound(err) {
		logger.Info("firewall group not found")

		nsxtFirewallGroupConfig := typesv56.NsxtFirewallGroup{
			Name:      ownerCluster.Spec.ControlPlaneRef.Name,
			TypeValue: typesv56.FirewallGroupTypeIpSet,
			OwnerRef: &typesv56.OpenApiReference{
				Name: nsxtEdgeGateway.EdgeGateway.Name,
				ID:   nsxtEdgeGateway.EdgeGateway.ID,
			},
		}

		nsxtFirewallGroup, err = vdc.CreateNsxtFirewallGroup(&nsxtFirewallGroupConfig)
		if err != nil {
			logger.Error(err, "error creating nsxt firewall group")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	nsxtAlbPool, err := vcdClient.GetAlbPoolByName(nsxtEdgeGateway.EdgeGateway.ID, ownerCluster.Spec.ControlPlaneRef.Name)
	if govcd.ContainsNotFound(err) {
		logger.Info("alb pool not found")

		albPoolConfig := typesv56.NsxtAlbPool{
			Name:        ownerCluster.Spec.ControlPlaneRef.Name,
			DefaultPort: ptr(6443),
			GatewayRef: typesv56.OpenApiReference{
				Name: nsxtEdgeGateway.EdgeGateway.Name,
				ID:   nsxtEdgeGateway.EdgeGateway.ID,
			},
			MemberGroupRef: &typesv56.OpenApiReference{
				Name: nsxtFirewallGroup.NsxtFirewallGroup.Name,
				ID:   nsxtFirewallGroup.NsxtFirewallGroup.ID,
			},
			HealthMonitors: []typesv56.NsxtAlbPoolHealthMonitor{
				{
					Type: "TCP",
				},
			},
		}

		createdNsxtAlbPool, err := vcdClient.CreateNsxtAlbPool(&albPoolConfig)
		if err != nil {
			logger.Error(err, "error creating nsxt alb pool")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		nsxtAlbPool = createdNsxtAlbPool
	}

	logger.Info("get virtual service by name", "edgeGateway", nsxtEdgeGateway.EdgeGateway.ID, "controlPlaneRef", ownerCluster.Spec.ControlPlaneRef.Name)

	nsxtAlbVirtualService, err := vcdClient.GetAlbVirtualServiceByName(nsxtEdgeGateway.EdgeGateway.ID, ownerCluster.Spec.ControlPlaneRef.Name)
	if vcdutil.IgnoreNotFound(err) != nil {
		logger.Error(err, "error getting virual service")

		return ctrl.Result{}, err
	}

	if govcd.ContainsNotFound(err) {
		queryParameters := url.Values{}
		queryParameters.Set("filter", "gatewayRef.id=="+nsxtEdgeGateway.EdgeGateway.ID)

		serviceEngineGroupAssignment, err := vcdClient.GetFilteredAlbServiceEngineGroupAssignmentByName(tenantCluster.Spec.ServiceEngineGroup, queryParameters)
		if err != nil {
			logger.Error(err, "error getting service engine group assignment")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.Info("creating virtual service")

		nsxtAlbVirtualServiceConfig := typesv56.NsxtAlbVirtualService{
			Name:    ownerCluster.Spec.ControlPlaneRef.Name,
			Enabled: ptr(true),
			GatewayRef: typesv56.OpenApiReference{
				Name: nsxtEdgeGateway.EdgeGateway.Name,
				ID:   nsxtEdgeGateway.EdgeGateway.ID,
			},
			LoadBalancerPoolRef: typesv56.OpenApiReference{
				Name: nsxtAlbPool.NsxtAlbPool.Name,
				ID:   nsxtAlbPool.NsxtAlbPool.ID,
			},
			ServiceEngineGroupRef: typesv56.OpenApiReference{
				Name: serviceEngineGroupAssignment.NsxtAlbServiceEngineGroupAssignment.ServiceEngineGroupRef.Name,
				ID:   serviceEngineGroupAssignment.NsxtAlbServiceEngineGroupAssignment.ServiceEngineGroupRef.ID,
			},
			VirtualIpAddress: tenantCluster.Spec.ControlPlaneEndpoint.Host,
			ServicePorts: []typesv56.NsxtAlbVirtualServicePort{
				{
					PortStart: ptr(int(tenantCluster.Spec.ControlPlaneEndpoint.Port)),
					TcpUdpProfile: &typesv56.NsxtAlbVirtualServicePortTcpUdpProfile{
						Type: "TCP_PROXY",
					},
				},
			},
			ApplicationProfile: typesv56.NsxtAlbVirtualServiceApplicationProfile{
				Type: "L4",
			},
		}

		nsxtAlbVirtualService, err = vcdClient.CreateNsxtAlbVirtualService(&nsxtAlbVirtualServiceConfig)
		if err != nil {
			logger.Error(err, "error creating nsxt alb virtual service")

			conditions.MarkFalse(&tenantCluster, tenantv1.VirtualServiceReadyCondition, tenantv1.VirtualServiceCreateFailedReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	logger.Info("reconciled virtual service", "id", nsxtAlbVirtualService.NsxtAlbVirtualService.ID)

	vApp, err := vdc.GetVAppByName(tenantCluster.Name, true)
	if vcdutil.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	if govcd.ContainsNotFound(err) {
		vApp, err = vdc.CreateRawVApp(tenantCluster.Name, "")
		if err != nil {
			logger.Error(err, "error creating raw vapp")

			conditions.MarkFalse(&tenantCluster, tenantv1.VAppReadyCondition, tenantv1.VAppCreateFailedReason, clusterv1.ConditionSeverityError, err.Error())

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		orgVDCNetwork, err := vdc.GetOrgVdcNetworkByName(tenantCluster.Spec.Network, true)
		if err != nil {
			logger.Error(err, "error getting org vdc network")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		_, err = vApp.AddOrgNetwork(&govcd.VappNetworkSettings{}, orgVDCNetwork.OrgVDCNetwork, false)
		if err != nil {
			logger.Error(err, "error adding vapp org network")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		task, err := vApp.PowerOn()
		if err != nil {
			logger.Error(err, "error powering on vapp")

			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}

		logger.Info("waiting for vapp power on task", "id", task.Task.ID)

		err = task.WaitTaskCompletion()
		if err != nil {
			logger.Error(err, "error waiting for task completion")
		}

		logger.Info("vapp power on task completed", "id", task.Task.ID)
	}

	logger.Info("reconciled vapp", "id", vApp.VApp.ID)

	tenantCluster.Status.VApp = &tenantv1.CloudDirectorReference{
		ID:   vApp.VApp.ID,
		Name: vApp.VApp.Name,
	}

	conditions.MarkTrue(&tenantCluster, tenantv1.VAppReadyCondition)

	tenantCluster.Status.VirtualService = &tenantv1.CloudDirectorReference{
		ID:   nsxtAlbVirtualService.NsxtAlbVirtualService.ID,
		Name: nsxtAlbVirtualService.NsxtAlbVirtualService.Name,
	}

	conditions.MarkTrue(&tenantCluster, tenantv1.VirtualServiceReadyCondition)

	tenantCluster.Status.Pool = &tenantv1.CloudDirectorReference{
		ID:   nsxtAlbPool.NsxtAlbPool.ID,
		Name: nsxtAlbPool.NsxtAlbPool.Name,
	}

	conditions.MarkTrue(&tenantCluster, tenantv1.PoolReadyCondition)

	tenantCluster.Status.IPSet = &tenantv1.CloudDirectorReference{
		ID:   nsxtFirewallGroup.NsxtFirewallGroup.ID,
		Name: nsxtFirewallGroup.NsxtFirewallGroup.Name,
	}

	conditions.MarkTrue(&tenantCluster, tenantv1.IPSetReadyCondition)

	tenantCluster.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *CloudDirectorTenantClusterReconciler) reconcileDelete(ctx context.Context, tenantCluster *tenantv1.CloudDirectorTenantCluster, ownerCluster *clusterv1.Cluster) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	machines, err := collections.GetFilteredMachinesForCluster(ctx, r.Client, ownerCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(machines) > 0 {
		logger.Info("ignoring until all machines are deleted", "machines", len(machines))

		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	vcdClient, err := vcdutil.GetVCDClientFromTenantCluster(ctx, r.Client, tenantCluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	org, err := vcdClient.GetOrgByName(tenantCluster.Spec.Organization)
	if err != nil {
		return ctrl.Result{}, err
	}

	vdc, err := org.GetVDCByName(tenantCluster.Spec.VirtualDataCenter, false)
	if err != nil {
		return ctrl.Result{}, err
	}

	if tenantCluster.Status.VirtualService != nil {
		virtualService, err := vcdClient.GetAlbVirtualServiceById(tenantCluster.Status.VirtualService.ID)
		if vcdutil.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		if !govcd.ContainsNotFound(err) {
			err := virtualService.Delete()
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if tenantCluster.Status.Pool != nil {
		albPool, err := vcdClient.GetAlbPoolById(tenantCluster.Status.Pool.ID)
		if vcdutil.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		if !govcd.ContainsNotFound(err) {
			err := albPool.Delete()
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if tenantCluster.Status.IPSet != nil {
		firewallGroup, err := vdc.GetNsxtFirewallGroupById(tenantCluster.Status.IPSet.ID)
		if vcdutil.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		if !govcd.ContainsNotFound(err) {
			if len(firewallGroup.NsxtFirewallGroup.IpAddresses) > 0 {
				logger.Info("ignoring cluster with ip addresses in firewall group")

				return ctrl.Result{}, nil
			}

			err := firewallGroup.Delete()
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if tenantCluster.Status.VApp != nil {
		vApp, err := vdc.GetVAppById(tenantCluster.Status.VApp.ID, false)
		if vcdutil.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		if !govcd.ContainsNotFound(err) {
			if vApp.VApp.Children != nil {
				logger.Info("vapp", "children", len(vApp.VApp.Children.VM))
			}

			status, err := vApp.GetStatus()
			if err != nil {
				return ctrl.Result{}, err
			}

			if status != "POWERED_OFF" {
				task, err := vApp.Undeploy()
				if err != nil {
					return ctrl.Result{}, err
				}

				err = task.WaitTaskCompletion()
				if err != nil {
					return ctrl.Result{}, err
				}
			}

			task, err := vApp.Delete()
			if err != nil {
				return ctrl.Result{}, err
			}

			err = task.WaitTaskCompletion()
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	controllerutil.RemoveFinalizer(tenantCluster, CloudDirectorTenantClusterFinalizer)

	return ctrl.Result{}, nil
}

func (r *CloudDirectorTenantClusterReconciler) SetupWithManager(manager ctrl.Manager) error {
	scheme := manager.GetScheme()

	_ = tenantv1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	err := ctrl.NewControllerManagedBy(manager).
		For(&tenantv1.CloudDirectorTenantCluster{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      o.GetName(),
							Namespace: o.GetNamespace(),
						},
					},
				}
			}),
		).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}

func ptr[T any](t T) *T {
	return &t
}

func patchTenantCluster(ctx context.Context, patchHelper *patch.Helper, tenantCluster *tenantv1.CloudDirectorTenantCluster, opts ...patch.Option) error {
	conditions.SetSummary(tenantCluster, conditions.WithConditions(
		tenantv1.VirtualServiceReadyCondition,
		tenantv1.PoolReadyCondition,
		tenantv1.IPSetReadyCondition,
		tenantv1.VAppReadyCondition,
		tenantv1.ExternalIPAddressReady,
	))

	opts = append(opts,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			tenantv1.VirtualServiceReadyCondition,
			tenantv1.PoolReadyCondition,
			tenantv1.IPSetReadyCondition,
			tenantv1.VAppReadyCondition,
			tenantv1.ExternalIPAddressReady,
		}},
	)

	return patchHelper.Patch(ctx, tenantCluster, opts...)
}
