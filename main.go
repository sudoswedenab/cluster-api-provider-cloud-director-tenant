package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/go-logr/logr"
	"github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/controllers"
	"github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/util/clientcache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slogr := logr.FromSlogHandler(logger.Handler())

	ctrl.SetLogger(slogr)

	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error("error getting config", "err", err)

		os.Exit(1)
	}

	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		logger.Error("error creating manager", "err", err)

		os.Exit(1)
	}

	clientCache := clientcache.NewClientCache()

	err = (&controllers.CloudDirectorTenantClusterReconciler{
		Client:      mgr.GetClient(),
		ClientCache: clientCache,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating cluster controller", "err", err)

		os.Exit(1)
	}

	err = (&controllers.CloudDirectorTenantMachineReconciler{
		Client:      mgr.GetClient(),
		ClientCache: clientCache,
	}).SetupWithManager(mgr)
	if err != nil {
		logger.Error("error creating machine controller", "err", err)

		os.Exit(1)
	}

	err = mgr.Start(ctx)
	if err != nil {
		logger.Error("error running manager", "err", err)

		os.Exit(1)
	}
}
