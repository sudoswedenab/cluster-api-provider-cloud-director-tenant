REGISTRY ?= ghcr.io/sudoswedenab
IMAGE_NAME ?= capi-cloud-director-tenant
TAG ?= dev

RELEASE_DIR ?= _out

.PHONY: $(RELEASE_DIR)
$(RELEASE_DIR):
	mkdir --parents $(RELEASE_DIR)

.PHONY: clean-release
clean-release:
	rm --force kustomization.yaml
	rm --recursive --force $(RELEASE_DIR)

.PHONY: release
release: clean-release $(RELEASE_DIR)
	kustomize create --resources config/default
	kustomize edit set image controller=$(REGISTRY)/$(IMAGE_NAME):$(TAG)
	kustomize build > $(RELEASE_DIR)/infrastructure-components.yaml
