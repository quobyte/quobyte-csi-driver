Quobyte CSI is deployed with the following different kinds of pods.

* [Controller pod](_quobyte_csi_controller_pod.tpl)
  * This runs on a single node and starts different containers based on feature flags
  * It can talk to Quobyte API and execute API calls such as volume provisioning, expansion etc
* [NodeDriver pod](_quobyte_csi_node_driver_pod.tpl)
  * This runs a pod on every k8s-node (unless taints are configured)
  * It is responsible for mounting volumes into pods
  * It cannot talk to Quobyte API (CSI spec restriction)
  * (Application) Pod killer runs in the node driver pod
