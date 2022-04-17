# ingress-envoy

## Getting started


```sh
make release
```

```sh
kubectl apply -f out/ingress-envoy.yaml
```

## ensure that it works

```sh
kubectl apply -f samples/test_app.yaml
kubectl apply -f samples/test_ingress.yaml
```

```sh
kubectl port-forward svc/ingress-envoy -n ingress-envoy-system 80:50080
```

```sh
curl http://localhost:50080/Ingress_HTTP
```


