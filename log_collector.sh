#!/bin/bash

DRIVER_NAMESPACE=${DRIVER_NAMESPACE:-"kube-system"}

echo "Collecting Quobyte CSI Driver from the $DRIVER_NAMESPACE namespace..."

csi_pods=()
while IFS= read -r line; do
    csi_pods+=( "$line" )
done < <(kubectl -n $DRIVER_NAMESPACE get po -owide | grep ^quobyte-csi | cut -f 1 -d' ')

if [[ ${#csi_pods[@]} == 0 ]]; then
  echo "Quobyte CSI pods are not found under $DRIVER_NAMESPACE namespace."
  exit 1
fi

if [ -d csi_logs ]; then
  rm -rf csi_logs
fi

mkdir -p ./csi_logs
echo '###kubectl version info###' >> ./csi_logs/csi_pods.txt
kubectl version >> ./csi_logs/csi_pods.txt
echo '' >> ./csi_logs/csi_pods.txt
echo '###CSIDriver object status###' >> ./csi_logs/csi_pods.txt
kubectl get CSIDriver | grep ^csi.quobyte.com >> ./csi_logs/csi_pods.txt
echo '' >> ./csi_logs/csi_pods.txt
echo '###Quobyte CSI pods status###' >> ./csi_logs/csi_pods.txt
kubectl -n $DRIVER_NAMESPACE get po -owide | grep ^quobyte-csi >> ./csi_logs/csi_pods.txt

pod_killer_cache_pod=$(kubectl -n $DRIVER_NAMESPACE get po | grep "quobyte-csi-pod-killer-cache" | awk '{print $1}')
if [[ ! -z $pod_killer_cache_pod ]]; then
  kubectl -n $DRIVER_NAMESPACE logs $pod_killer_cache_pod >> ./csi_logs/$pod_killer_cache_pod.log
fi

for el in "${csi_pods[@]}"
do
  mkdir -p "./csi_logs/$el"
  if [[ $el =~ quobyte-csi-controller.* ]]; then
    kubectl -n $DRIVER_NAMESPACE logs $el -c csi-provisioner >> ./csi_logs/$el/csi-provisioner.log
    kubectl -n $DRIVER_NAMESPACE logs $el -c csi-attacher >> ./csi_logs/$el/csi-attacher.log
    kubectl -n $DRIVER_NAMESPACE logs $el -c csi-resizer >> ./csi_logs/$el/csi-resizer.log
    kubectl -n $DRIVER_NAMESPACE logs $el -c quobyte-csi-driver >> ./csi_logs/$el/quobyte-csi-driver.log
    kubectl -n $DRIVER_NAMESPACE logs $el -c csi-snapshotter >> ./csi_logs/$el/csi-snapshotter.log
  elif [[ $el =~ quobyte-csi-node.* ]];then
    kubectl -n $DRIVER_NAMESPACE logs $el -c csi-node-driver-registrar >> ./csi_logs/$el/csi-node-driver-registrar.log
    kubectl -n $DRIVER_NAMESPACE logs $el -c quobyte-csi-driver  >> ./csi_logs/$el/quobyte-csi-node-driver.log
    kubectl -n $DRIVER_NAMESPACE logs $el -c quobyte-csi-mount-monitor  >> ./csi_logs/$el/quobyte-csi-mount-monitor.log
 fi
done

# when snapshots are enabled we should also get logs from snapshot-controller
kubectl -n kube-system get po | grep -q "snapshot-controller"
found_snapshot_controller="$?"
if [[ ${found_snapshot_controller} -eq 0 ]]; then
  kubectl -n kube-system logs snapshot-controller-0 >> ./csi_logs/snapshot-controller.log
fi

# TODO(venkat): collect daemonset, statefulsets and deployment details as part of logs

if [[ -f quobyte_csi_logs.tar.gz ]]; then
 rm quobyte_csi_logs.tar.gz
fi

tar -zcf quobyte_csi_logs.tar.gz ./csi_logs
