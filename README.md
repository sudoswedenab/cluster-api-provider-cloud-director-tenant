# Kubernetes Cluster API Provider Cloud Director (Tenant)

Kubernetes-native declarative infrastructure for Cloud Director.

This is an experimental alternative to the [VMWare Cloud Director provider](https://github.com/vmware/cluster-api-provider-cloud-director).

The main limitation of the VMWare provider is that it requires system administrator rights to setup.

## Features

- Requires only tenant level permissions for managing clusters.
- Does not utilize Tanzu or Runtime Defined Entity Types for operation.
- Does not require specific bootstrap providers.
