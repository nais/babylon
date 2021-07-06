PHONY: all
.DEFAULT_GOAL := deploy-local

deploy-local: docker-build
	kubectl delete -f minikube.yaml
	kubectl apply -f minikube.yaml

docker-build:
	docker build -t babylon .

lint:
	golangci-lint run --fix

hooks:
    pre-commit install