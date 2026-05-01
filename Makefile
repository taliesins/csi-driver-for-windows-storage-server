
# Project Makefile for csi-driver-for-windows-storage-server

APP_NAME ?= csiplugin
REGISTRY ?= ghcr.io
IMAGE_NAME ?= taliesins/csi-driver-for-windows-storage-server
IMAGE_TAG ?= latest
PLATFORM ?= linux/amd64
CHART_NAME ?= csi-driver-for-windows-storage-server
CHART_PATH ?= chart/$(CHART_NAME)
CHART_OUT ?= chart/dist
CHART_VERSION ?= 0.1.0
APP_VERSION ?= $(IMAGE_TAG)
HELM_REGISTRY ?= oci://$(REGISTRY)/taliesins/helm

.PHONY: all build test integration-test image image-push chart-lint chart-package chart-push release lint pre-commit clean

all: build

build:
	go build -mod=mod -o bin/$(APP_NAME) ./cmd/csiplugin

test:
	go test -mod=mod ./...

integration-test:
	go test -mod=mod -tags integration ./pkg/iscsi

image:
	docker buildx build --platform=$(PLATFORM) -t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .

image-push:
	docker buildx build --platform=$(PLATFORM) -t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) --push .

chart-lint:
	helm lint $(CHART_PATH)

chart-package:
	rm -rf $(CHART_OUT)
	mkdir -p $(CHART_OUT)
	helm package $(CHART_PATH) --destination $(CHART_OUT) --version $(CHART_VERSION) --app-version $(APP_VERSION)

chart-push: chart-package
	helm push $(CHART_OUT)/$(CHART_NAME)-$(CHART_VERSION).tgz $(HELM_REGISTRY)

release:
	goreleaser release --clean

lint:
	pre-commit run --all-files

pre-commit:
	pre-commit install

clean:
	rm -rf bin/
	rm -rf $(CHART_OUT)
	go clean -mod=mod -r -x
