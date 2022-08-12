# Secure storage access using Pod Security Policy (PSP)

[Pod Security Policy](https://kubernetes.io/docs/concepts/policy/pod-security-policy/)
 can be used to control the security aspects of the pod deployments. This document
 walks you through an example deployment using
 [nginx unprivileged container](https://github.com/nginxinc/docker-nginx-unprivileged).

## Requirements

1. Kubernetes v1.14 or above.

   * On Kubernetes versions lower than v1.14 `runAsGroup` [does not work](https://github.com/kubernetes/enhancements/issues/213)

2. Quobyte CSI driver deployment with [PSP policies](../deploy/csi-driver-k8sv1.14-PSP.yaml).

3. `PodSecurityPolicy` admission plugin must be enabled.
 Edit `/etc/kubernetes/manifests/kube-apiserver.yaml` on master nodes and append `--enable-admission-plugins`
 with PodSecurityPolicy. After that, restart the nodes or kube-apiserver pods.

4. User and Group specified in PSP must exist on the host nodes.

5. **Nginx PSP Demo pod requires**

    * Hosts with nginx user (UID: 5050) and group (GID:5050). These UID and GID are used in the [example PSP](../example/psp/psp-example-definition.yaml).
     Create nginx user and group.

        ```bash
        sudo groupadd -g 5050 nginx; sudo useradd -u 5050 -g 5050 nginx
        ```  

    * Volume with at least read and execute permissions for the `nginx` user.  Volume permissions
     can be configured in StorageClass as `accessMode` for dynamically provisioned volumes.

6. All the example commands should be executed from
 the root directory of Quobyte CSI. Please get the Quobyte CSI example files and change to root directory.

    ```bash
      git clone https://github.com/quobyte/quobyte-csi.git
      cd quobyte-csi
    ````

## PSP example

Let us dive in and create an example PSP with restricted access. Using the example psp, we can
 deploy unprivileged nginx pod.

1. Create `quobyte` namespace

    ```bash
    kubectl create ns quobyte
    ```

2. Create Quobyte [admin secret](../example/quobyte-admin-credentials.yaml) (credentials are required for dynamic volume provision)

    ```bash
    kubectl create -f example/quobyte-admin-credentials.yaml
    ```

3. Review and create [storage class](../example/psp/StorageClass-PSP.yaml)

    ```bash
    kubectl create -f example/psp/StorageClass-PSP.yaml
    ```

4. Create a namespace `psp-example` to run the nginx pod nginx user

    ```bash
    kubectl create namespace psp-example
    ```

5. Create a service account `psp-user` in `psp-example` namespace

    ```bash
    kubectl create serviceaccount -n psp-example psp-user
    ```

6. Create aliases for kubectl commands. `kubectl-admin` is the admin user and
 `kubectl-user` is the service account `psp-user` in the namespace `psp-example`.

    ```bash
    # Admin user in the namespace "psp-example"
    alias kubectl-admin='kubectl -n psp-example'
    # psp-user in the namespace "psp-example"
    alias kubectl-user='kubectl --as=system:serviceaccount:psp-example:psp-user -n psp-example'
    ```

7. Update UID and GID in [example PSP definition](../example/psp/psp-example-definition.yaml) and create
 PSP.

    ```bash
    kubectl create -f example/psp/psp-example-definition.yaml
    ```

8. Create [Role and RoleBindings](../example/psp/psp-example-roles.yaml) for the `psp-user` in `psp-example` namespace

    ```bash
    kubectl-admin create -f example/psp/psp-example-roles.yaml
    ```

9. Verify `psp-user` can access the pod security policy `example-psp`

    ```bash
    kubectl-user auth can-i use psp/example-psp
    ```

    The above command should output `yes` for user to be able to deploy pods.

10. Create [PVC](../example/psp/pvc-dynamic-provision-psp.yaml)

    ```bash
    kubectl-user create -f example/psp/pvc-dynamic-provision-psp.yaml
    ```

11. Create [Pod](../example/psp/nginx-demo-pod-with-psp.yaml) with the created PVC

    ```bash
    kubectl-user create -f example/psp/nginx-demo-pod-with-psp.yaml
    ```

12. Wait for the pod to be in running state

    ```bash
    kubectl get po -w | grep 'nginx-psp-demo'
    ```

13. Verify user UID/GID inside created pod

      ```bash
      kubectl-admin exec -it nginx-psp-demo -- id
     ```

14. Copy [index file](../example/index.html) into the pod

    Unfortunately, `kubectl cp` does not work with non-root users. This should be done manually.

    Connect to the pod

      ```bash
      kubectl-admin exec -it nginx-psp-demo -- bash
      ```

    Create `index.html`

      ```bash
      cat > /usr/share/nginx/html/index.html <<EOF
      <!DOCTYPE html>
      <html>
      
      <head>
        <title>Welcome to Quobyte CSI!</title>
      </head>
      
      <body>
        <h1>Welcome to Quobyte CSI!</h1>
        <p>This file is retrieved from the mounted Quobyte volume.</p>

        <p><em>Thank you for using Quobyte.</em></p>
      </body>
     
      </html>
      EOF
     
     ```

    Please verify file permissions on the created index.html and exit from the pod.

      ```bash
      ls -l /usr/share/nginx/html/index.html
      exit
      ```

15. Access the index page from the command line

      ```bash
      curl http://$(kubectl-user get pods nginx-psp-demo -o yaml | grep 'podIP:' | awk '{print $2}'):8080
      ```

      The above command should retrieve the Quobyte CSI welcome page (in raw html format).
