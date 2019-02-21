# Install

```
$ ./cert-bootstrap.sh   
$ kubectl apply -f k8s/deployment.yaml
$ kubectl apply -f k8s/service.yaml  
$ ./webhook-patch-ca-bundle.sh | kubectl apply -f - 
```