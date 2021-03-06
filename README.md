# ingress-envoy

A very simple kubernetes ingress powered by envoy.

## Getting started

Build the release yaml:
```sh
make release
```

Apply to your cluster:
```sh
kubectl apply -f out/ingress-envoy.yaml
```

## Ensure that it works

Apply the test application and ingress resources:
```sh
kubectl apply -f samples/test_ingressclass.yaml
kubectl apply -f samples/test_app.yaml
kubectl apply -f samples/test_ingress.yaml
```

Start a port-forward to test the ingress locally.
```sh
kubectl port-forward svc/ingress-envoy-controller-manager -n ingress-envoy-system 8000:80
```

curl the test path:
```sh
curl http://localhost:8000/Ingress_HTTP
```

The response should look something like this:

```json
{
  "args": {}, 
  "headers": {
    "Accept": "*/*", 
    "Host": "127.0.0.1:8000", 
    "User-Agent": "curl/7.68.0", 
    "X-Envoy-Expected-Rq-Timeout-Ms": "15000", 
    "X-Envoy-Original-Path": "/Ingress_HTTP"
  }, 
  "origin": "10.42.0.41", 
  "url": "http://127.0.0.1:8000/get"
}
```

## Why build this?

On the upstream kubernetes docs [Kubernetes Ingress Controllers](https://kubernetes.io/docs/concepts/services-networking/ingress-controllers/#additional-controllers) there are over 20+ Ingress Controllers already posted. 5 of which are already using envoy as the loadbalaner/proxy. So why build another one?

1. To help bring understanding to envoy. When choosing a loadbalancer/proxy, envoy can seem like a more complex choice to HAProxy or nginx. It is possible that envoy brings extra complexity in some usecases, but it can also be used in the simple case. Looking at the envoy based ingress options for kubernetes, none of them seemed as simple as ingress-nginx. I figured why not take a swing at a building a simple ingress powered by envoy.

2. Self learning. Ingress is a very critical component for running production workloads in kubernetes. Envoy is quickly taking ground as the defacto proxy/loadbalancer for cloud native workloads. xDS is aimed at becoming the universal data plane API. This project seemed like a good way to gain some knowledge on how these pieces work together.


## Contributing

Please do. Just open an issue first and feel free to open a PR with the proposed changes.