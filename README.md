# SnapshotPolicy
A Kubernetes controller for use in conjuction with [VolumeSnapshot](https://github.com/cisco-sso/external-storage/tree/master/snapshot).  This controller will create and retain volumesnapshot objects as declared.

## Build
Check this out at $GOPATH/src/github.com/cisco-sso/snapshotpolicy
```
dep ensure
go build .
```
Or simply run `docker build .` anwywhere where this is checked out.


## Example SnapshotPolicy
[See artifacts/examples](artifacts/examples/example-policy.yaml)

```
apiVersion: snapshotpolicy.ciscosso.io/v1alpha1
kind: snapshotpolicy
metadata:
  name: demo-pvc-policy
spec:
  claims:
  - demo-pvc
  unit: minute
  period: 1
  retention: 3
  strategy:
    name: inuse
```

The above snapshot policy will create volume snapshots of the PersistentVolumeClaim named `demo-pvc` every `1` `minute` and retain `3` snapshots.  Each time the control loop is ran when `3` snapshots exist one will be deleted.

## Strategy InUse
A strategy has been added to perform no operation prior to creating the snapshot.  Annotations have been added in the following branch of volume snapshot to achieve this and only support the openstack provider.
https://github.com/cisco-sso/external-storage/tree/openstack-in-use-snapshots