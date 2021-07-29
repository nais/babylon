PHONY: all
SHELL=/bin/bash
.DEFAULT_GOAL := deploy-local

deploy-local:
	if [ "$(shell minikube status | grep host | rev | cut -d' ' -f1 | rev)" == "Stopped" ]; then minikube start; fi
	kubectx minikube
	# apply minikube docker variables and build image
	@eval $$(minikube docker-env --shell=bash); \
	$(MAKE) docker-build-fast
	kubectl delete -f minikube.yaml --context minikube || true
	kubectl apply -f minikube.yaml --context minikube

docker-build:
	docker build -t babylon .

docker-build-fast:
	GOOS=linux CGO_ENABLED=0 go build -o babylon
	docker build -f Local.dockerfile -t babylon .

lint:
	golangci-lint run --fix

hooks:
	pre-commit install

test: ## Run tests.
	if [ "$(shell minikube status | grep host | rev | cut -d' ' -f1 | rev)" == "Stopped" ]; then minikube start; fi
	kubectl delete deploy -n=babylon-test babylon-test --context=minikube || true
	kubectl wait deploy/babylon-test -n=babylon-test --context=minikube --for=delete || true
	kubectx minikube
	# apply minikube docker variables and build image
	@eval $$(minikube docker-env --shell=bash); \
	$(MAKE) docker-build-fast
	go test ./...
	kubectl kuttl test --timeout=60 --start-kind=false

uml: # generate uml diagram
	plantuml -tsvg docs/*.puml
