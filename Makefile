
# Project Makefile for csi-driver-iscsi-for-windows

APP_NAME ?= iscsiplugin
REGISTRY ?= test
IMAGE_NAME ?= $(APP_NAME)
IMAGE_TAG ?= latest
PLATFORM ?= linux/amd64

.PHONY: all build test image release lint pre-commit clean

all: build

build:
	go build -o bin/$(APP_NAME) ./cmd/iscsiplugin

test:
	go test ./...

image:
	docker buildx build --platform=$(PLATFORM) -t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .

release:
	goreleaser release --clean

lint:
	pre-commit run --all-files

pre-commit:
	pre-commit install

clean:
	rm -rf bin/
	go clean -mod=vendor -r -x
