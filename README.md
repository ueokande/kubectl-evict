# kubectl-evict

A kubectl plugin to evict pods

## Installation

```console
$ go install github.com/ueokande/kubectl-evict@latest
```

## Usage

Evict a pod nginx:
```console
$ kubectl evict nginx
```

Evict all pods defined by label app=nginx:
```console
$ kubectl evict -l app=nginx
```

Evict all pods from of a deployment named nginx:
```console
$ kubectl evict deployment/nginx -c nginx-1
```

Evict all pods from node worker-1:
```console
$ kubectl evict node/worker-1
```

## Developing

Create a cluster:

```console
$ kind create cluster --config .kind/cluster.yaml
```

Then deploy nginx with a Deployment and PodDIsruptionBudget:

```console
$ kubectl apply -f .kind/deployment.yaml -f .kind/pdb.yaml
```

## LICENSE

[MIT](./LICENSE)
