# Client update example

Before upgrading Quobyte client on a k8s node, all application pods must be drained from the
  node. Otherwise, application pods will be left with unclean mount and needs a restart.

Before proceeding any furhther, please read k8s documentation on [drain](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node/), [PodDisruptionBudget](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/)

The following example is based on containerized Quobyte client. If you are using Quobyte native
  client you should follow same procedure but need to upgrade Quobyte client between drain and
  uncordon phases in instruction 3.

**All commands in this document are for example purposes and MUST ONLY be used with proper care
   and adjustments to your environments.**

1. Update Quobyte client daemonset and set `spec.updateStrategy` to `onDelete`
     and apply the change

    ```yaml
    spec:
        selector:
        matchLabels:
            role: client
        updateStrategy:
        type: OnDelete
        template:
    ```

2. Update client daemonset container image and apply the change.

3. Drain all application pods on the node, delete quobyte client,
   let the daemonset recreate quobyte client pod on delete and uncordon node
   for further pod schedule once the new quobyte client is scheduled.

    **The below script is only for example purposes and should be modified as required for your setup before use**

    ```bash
        #!/bin/bash

        QUOBYTE_CLIENT_NAMESPACE=""
        QUOBYTE_CLIENT_DS_NAME="client"

        for node in $(kubectl get nodes | awk 'NR>1 { print $1}'); do
          # optionally, you can configure your apps with distinctive lables (for example, storage-type: quobyte)
          # and drain only those pods, see kubectl drain -h for more info
          kubectl drain $node --ignore-daemonsets \
            --delete-local-data \
            --force=true
          drain_status="$?"
          if [[ ${drain_status} -eq 0 ]]; then
            if [[ ! -z "$QUOBYTE_CLIENT_NAMESPACE" ]]; then
              quobyte_client_namespace="-n $QUOBYTE_CLIENT_NAMESPACE"
            else
              quobyte_client_namespace=""
            fi
            client_on_node="$(kubectl get po ${quobyte_client_namespace} \
              -owide | grep -E "${QUOBYTE_CLIENT_DS_NAME}.*$node" | awk '{print $1}')"
            echo "deleting Quobyte client $client_on_node on node"
            kubectl delete po $client_on_node
            echo "Waiting for the new Quobyte client to be created..."
            until kubectl get po "$(kubectl get po ${quobyte_client_namespace} \
              -owide | grep -E "${QUOBYTE_CLIENT_DS_NAME}.*$node" | awk '{print $1}')" \
              | grep -m 1 "Running"; do sleep 2 ; done
            echo "Quobyte client recreated on node ${node}"
          else
            echo "Draining of node $node failed. Quobyte client is not updated on this node. Retry"
            echo "  after fixing the reported issues"
          fi
          kubectl uncordon $node
        done
    ```
