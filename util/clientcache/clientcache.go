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
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
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
	mutex         sync.Mutex
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

	p.mutex.Lock()
	defer p.mutex.Unlock()

	i, found := p.expiringCache.Get(sum)
	if found {
		logger.Info("existing client found", "sum", sum)

		c := i.(*govcd.VCDClient)

		token := c.Client.VCDToken
		remainingTTL, err := getTokenRemainingTTL(token)
		if err != nil {
			return nil, err
		}

		clientCacheKeyTTL.WithLabelValues(
			fmt.Sprintf("%d", sum),
			string(endpoint),
			string(organization),
		).Set(remainingTTL.Seconds())

		return c, nil
	}

	logger.Info("creating new client", "sum", sum)
	vcdClient := govcd.NewVCDClient(*u, false)
	token, err := vcdClient.SetApiToken(string(organization), string(apiToken))
	if err != nil {
		return nil, err
	}

	ttl, err := getTokenRemainingTTL(token.AccessToken)
	if err != nil {
		return nil, err
	}

	p.expiringCache.Add(sum, vcdClient, ttl)
	clientCacheKeyTTL.WithLabelValues(
		fmt.Sprintf("%d", sum),
		string(endpoint),
		string(organization),
	).Set(ttl.Seconds())

	return vcdClient, nil
}

var _ ClientCache = &clientCacheProvider{}

func getTokenRemainingTTL(tokenString string) (time.Duration, error) {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return time.Duration(-1), errors.New("Unable to parse input as JWT token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Duration(-1), errors.New("Unable to parse token claims")
	}

	expFloat, ok := claims["exp"].(float64)
	if !ok {
		return time.Duration(-1), errors.New("Unable to get expiration date from token")
	}

	expiration := time.Unix(int64(expFloat), 0)

	return time.Until(expiration), nil
}

func NewClientCache() *clientCacheProvider {
	c := clientCacheProvider{
		expiringCache: cache.NewLRUExpireCache(10),
	}

	return &c
}
