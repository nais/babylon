# Babylon - Project Gardener [![build](https://github.com/nais/babylon/actions/workflows/pipeline.yaml/badge.svg)](https://github.com/nais/babylon/actions/workflows/pipeline.yaml) [![CodeQL](https://github.com/nais/babylon/actions/workflows/codeql.yaml/badge.svg)](https://github.com/nais/babylon/actions/workflows/codeql.yaml) [![Go Report Card](https://goreportcard.com/badge/github.com/nais/babylon)](https://goreportcard.com/report/github.com/nais/babylon)

## About

By default, the application will not perform destructive actions. To arm it, either set the `-armed`-flag 
or set the `ARMED` 💥 environment variable to true. 


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

### Integration tests with `kuttl`

We have set up integration tests using `kuttl`. The tests are found in [`tests/e2e`](tests/e2e), 
see `kuttl`'s [documentation](https://kuttl.dev/docs/). All tests will have a running instance of babylon
in the background, as specified in [`tests/before/babylon.yaml`](tests/before/babylon.yaml). 

Tests work by specifying a cluster configuration, and then performing assertions on that configuration.
For example asserting that babylon has deleted some kind of resource.

### Setup kuttl

#### Automatically

```shell
$ make test
```

#### Manually

```sh
# install packages
$ brew tap kudobuilder/tap
$ brew install kuttl-cli

# run integration tests with kubernetes-in-docker (KIND)
$ kubectl kuttl test

# or you can run integration tests with minikube
$ minikube start
$ kubectl kuttl test --start-kind=false
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
