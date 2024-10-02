package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen crd object rbac:roleName=manager-role paths="./..."
