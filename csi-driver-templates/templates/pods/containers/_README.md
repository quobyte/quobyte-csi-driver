Container definitions (file names ending with `_container.tpl` convention) are partial and these cannot be created directly as k8s as resources/types. These are `include` resources that compose pods. Hence, these definitions should be atomic units and must not contain `---`.

See [_quobyte_csi_controller_pod.tpl](../_quobyte_csi_controller_pod.tpl) and [_quobyte_csi_node_driver_pod.tpl](../_quobyte_csi_node_driver_pod.tpl) to find out the usage of containers elements in `quobyte-csi-driver` driver definition.

Changing `rbac`, `volumes`, `serviceAccount` details in containers require appropriate change in the corresponding resource definitions.
