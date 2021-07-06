PHONY: all
.DEFAULT_GOAL := deploy-local

deploy-local: docker-build
	if [ "$(shell minikube status | grep host | rev | cut -d' ' -f1 | rev)" == "Stopped" ]; then minikube start; fi
	@eval $$(minikube docker-env --shell=bash); \
	kubectl delete -f minikube.yaml; \
	kubectl apply -f minikube.yaml

docker-build:
	docker build -t babylon .

lint:
	golangci-lint run --fix

hooks:
	pre-commit install
