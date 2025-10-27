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

	clientCache.RegisterMetrics()

	err = mgr.Start(ctx)
	if err != nil {
		logger.Error("error running manager", "err", err)

		os.Exit(1)
	}
}
