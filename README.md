# local-csi-driver

Kubernetes CSI driver example using local storage similar to [local volumes](https://kubernetes.io/docs/concepts/storage/volumes/#local)

This CSI-Driver does no [Dynamic Volume Provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/) so you still have to create the PV yourself as shown in [test-pod.yaml](test-pod.yaml)

Unlike the Kubernetes local volumes this driver creates the storage directory automatically if missing but won't remove it again. The main storage path on the node (host) is configured in the [csi-driver.yaml](csi-driver.yaml) towards the end of the file.
```yaml
        - name: storage-dir
          hostPath:
            path: /mnt/storage/
            type: Directory
```

The persistent volumes are all created in this directory (flat, no hierarchy) with the name derived from the PV, see [test-pod.yaml](test-pod.yaml).

To develop and test this locally with minikube use the following commands.
```
to compile locally:
go build -o local-csi-driver main.go

to build the docker container
docker build -t local-csi-driver:local .

to load the image into minikube:
minikube image load local-csi-driver:local
(fix the csi-driver.yaml to point to image local-csi-driver:local)
kubectl apply -f csi-driver.yaml
kubectl apply -f test-pod.yaml
```

Follow logs of the CSI driver with:
```
kubectl -n kube-system logs -f $(kubectl -n kube-system get pod -l app=local-csi-driver -o name) -c local-csi-driver
```

Related commands
```
kubectl get CSIDriver
kubectl get CSINode
kubectl get StorageClass
```
