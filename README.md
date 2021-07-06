# Babylon - Project Gardener [![build](https://github.com/nais/babylon/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/nais/babylon/actions/workflows/pipeline.yaml) [![CodeQL](https://github.com/nais/babylon/actions/workflows/codeql.yaml/badge.svg)](https://github.com/nais/babylon/actions/workflows/codeql.yaml) [![Go Report Card](https://goreportcard.com/badge/github.com/nais/babylon)](https://goreportcard.com/report/github.com/nais/babylon)

## Milestones

- [ ] Deploy app on NAIS
- [ ] Fetch a K8s resource
- [ ] Delete a K8s resource  
- [ ] Define criteria for service deletion
- [ ] Send slack message to the creators of application

## Using `make`

```shell
# To build and deploy
$ make # make deploy-local works too
# To check linting
$ make lint
```

## Local kubernetes development 

```sh 
$ minikube start

$ eval $(minikube -p minikube docker-env)

$ docker build -t babylon .

$ kubectl apply -f minikube.yaml
```

### Access running application

```shell
$ minikube ip
192.168.64.2 # example, copy your own
$ sudo $EDITOR /etc/hosts
192.168.64.2 babylon.local
$ sudo killall -HUP mDNSResponder
```

### Developer setup

You must have pre-commit installed, then run `make hooks` to install git hooks. 