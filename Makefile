PHONY: all
SHELL=/bin/bash
.DEFAULT_GOAL := deploy-local

deploy-local:
	if [ "$(shell minikube status | grep host | rev | cut -d' ' -f1 | rev)" == "Stopped" ]; then minikube start; fi
	# apply minikube docker variables and build image
	@eval $$(minikube docker-env --shell=bash); \
	$(MAKE) docker-build; \
	kubectl delete -f minikube.yaml; \
	kubectl apply -f minikube.yaml

docker-build:
	docker build -t babylon .

lint:
	golangci-lint run --fix

hooks:
	pre-commit install

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: # lint ## Run tests.
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out