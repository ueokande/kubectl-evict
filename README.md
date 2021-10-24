# kubectl-evict

A kubectl plugin to evict pods

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
