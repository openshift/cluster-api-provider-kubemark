# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

DBG ?= 0

ifeq ($(DBG),1)
GOGCFLAGS ?= -gcflags=all="-N -l"
endif

VERSION     ?= $(shell git describe --always --abbrev=7)
MUTABLE_TAG ?= latest
IMAGE        = kubemark-machine-controllers

.PHONY: all
all: generate build images check

NO_DOCKER ?= 0
ifeq ($(NO_DOCKER), 1)
  DOCKER_CMD =
  IMAGE_BUILD_CMD = imagebuilder
  CGO_ENABLED = 1
else
  DOCKER_CMD := docker run --rm -e CGO_ENABLED=1 -v "$(PWD)":/go/src/github.com/openshift/cluster-api-provider-kubemark:Z -w /go/src/github.com/openshift/cluster-api-provider-kubemark openshift/origin-release:golang-1.10
  IMAGE_BUILD_CMD = docker build
endif

.PHONY: depend
depend:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure

.PHONY: vendor
vendor:
	dep version || go get -u github.com/golang/dep/cmd/dep
	dep ensure -v
	patch -p1 < 0001-Delete-annotated-machines-first-when-scaling-down.patch
	patch -p1 < 0002-Sort-machines-before-syncing.patch
	patch -p1 < 0001-Validate-machineset-before-reconciliation.patch
	patch -p1 < 0001-Upstream-677-Init-klog-in-manager-properly.patch

.PHONY: generate
generate:
	go install $(GOGCFLAGS) -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-kubemark/vendor/github.com/golang/mock/mockgen
	go generate ./pkg/... ./cmd/...

.PHONY: gendeepcopy
gendeepcopy:
	cd ./vendor/k8s.io/code-generator/cmd && go install ./deepcopy-gen
	deepcopy-gen \
	  -i ./pkg/apis/kubemarkproviderconfig/v1alpha1/ \
	  -O zz_generated.deepcopy \
	  -h boilerplate.go.txt

.PHONY: test
test: unit

bin:
	@mkdir $@

.PHONY: build
build: ## build binaries
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/manager -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-kubemark/cmd/manager
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/machine-controller-manager -ldflags '-extldflags "-static"' github.com/openshift/cluster-api-provider-kubemark/vendor/sigs.k8s.io/cluster-api/cmd/manager
	$(DOCKER_CMD) go test $(GOGCFLAGS) -c -o bin/test-e2e github.com/openshift/cluster-api-provider-kubemark/vendor/github.com/openshift/cluster-api-actuator-pkg/pkg/e2e


kubemark-actuator:
	$(DOCKER_CMD) go build $(GOGCFLAGS) -o bin/kubemark-actuator github.com/openshift/cluster-api-provider-kubemark/cmd/kubemark-actuator

.PHONY: images
images: ## Create images
	$(IMAGE_BUILD_CMD) -t "$(IMAGE):$(VERSION)" -t "$(IMAGE):$(MUTABLE_TAG)" ./

.PHONY: push
push:
	docker push "$(IMAGE):$(VERSION)"
	docker push "$(IMAGE):$(MUTABLE_TAG)"

.PHONY: deploy
deploy:
	./hack/deploy.sh

.PHONY: check
check: fmt vet lint test ## Check your code

.PHONY: unit
unit: # Run unit test
	$(DOCKER_CMD) go test -race -cover ./cmd/... ./pkg/...

.PHONY: lint
lint: ## Go lint your code
	hack/go-lint.sh -min_confidence 0.3 $$(go list -f '{{ .ImportPath }}' ./... | grep -v -e 'github.com/openshift/cluster-api-provider-kubemark/test' -e 'github.com/openshift/cluster-api-provider-kubemark/pkg/cloud/aws/client/mock')

.PHONY: fmt
fmt: ## Go fmt your code
	hack/go-fmt.sh .

.PHONY: vet
vet: ## Apply go vet to all go files
	hack/go-vet.sh ./...

.PHONY: test-e2e
test-e2e: ## Run e2e tests
	 go run ./test/e2e/*.go -alsologtostderr $${NAMESPACE:+--namespace=$${NAMESPACE}} $${FOCUS:+--focus=$${FOCUS}}

.PHONY: k8s-e2e
k8s-e2e: ## Run k8s specific e2e test
	go test -timeout 30m \
		-v ./vendor/github.com/openshift/cluster-api-actuator-pkg/pkg/e2e \
		-kubeconfig $${KUBECONFIG:-~/.kube/config} \
		-machine-api-namespace $${NAMESPACE:-kube-system} \
		-ginkgo.v \
		-args -v 5 -logtostderr true

.PHONY: help
help:
	@grep -E '^[a-zA-Z/0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
