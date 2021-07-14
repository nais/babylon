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

test: ## Run tests.
	if [ "$(shell minikube status | grep host | rev | cut -d' ' -f1 | rev)" == "Stopped" ]; then minikube start; fi
	# apply minikube docker variables and build image
	@eval $$(minikube docker-env --shell=bash); \
	$(MAKE) docker-build; \
	go test ./...; \
	kubectl kuttl test --timeout=120 --start-kind=false
