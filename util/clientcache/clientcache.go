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

package clientcache

import (
	"context"
	"hash/fnv"
	"net/url"
	"time"

	tenantv1 "github.com/sudoswedenab/cluster-api-provider-cloud-director-tenant/api/v1alpha1"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClientCache interface {
	GetVCDClient(context.Context, client.Client, *tenantv1.CloudDirectorTenantCluster) (*govcd.VCDClient, error)
}

type clientCacheProvider struct {
	expiringCache *cache.LRUExpireCache
}

func (p *clientCacheProvider) GetVCDClient(ctx context.Context, c client.Client, tenantCluster *tenantv1.CloudDirectorTenantCluster) (*govcd.VCDClient, error) {
	logger := ctrl.LoggerFrom(ctx)

	objectKey := client.ObjectKey{
		Name:      tenantCluster.Spec.IdentityRef.Name,
		Namespace: tenantCluster.Namespace,
	}

	var secret corev1.Secret
	err := c.Get(ctx, objectKey, &secret)
	if err != nil {
		return nil, err
	}

	endpoint, ok := secret.Data["url"]
	if !ok {
		return nil, nil
	}

	u, err := url.Parse(string(endpoint) + "/api")
	if err != nil {
		return nil, err
	}

	apiToken, ok := secret.Data["apiToken"]
	if !ok {
		return nil, nil
	}

	organization, ok := secret.Data["organization"]
	if !ok {
		return nil, nil
	}

	hash32 := fnv.New32()

	hash32.Write(endpoint)
	hash32.Write(apiToken)
	hash32.Write(organization)

	sum := hash32.Sum32()

	logger.Info("secret hashed", "secretName", secret.Name, "secretNamespace", secret.Namespace, "sum", sum)

	i, found := p.expiringCache.Get(sum)
	if found {
		logger.Info("existing client found", "sum", sum)

		return i.(*govcd.VCDClient), nil
	}

	logger.Info("creating new client", "sum", sum)

	vcdClient := govcd.NewVCDClient(*u, false)
	refreshToken, err := vcdClient.SetApiToken(string(organization), string(apiToken))
	if err != nil {
		return nil, err
	}

	p.expiringCache.Add(sum, vcdClient, time.Second*time.Duration(refreshToken.ExpiresIn))

	return vcdClient, nil
}

var _ ClientCache = &clientCacheProvider{}

func NewClientCache() *clientCacheProvider {
	c := clientCacheProvider{
		expiringCache: cache.NewLRUExpireCache(10),
	}

	return &c
}
