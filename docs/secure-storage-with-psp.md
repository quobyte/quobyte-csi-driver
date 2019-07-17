# Secure storage access using Pod Security Policy (PSP)

[Pod Security Policy](https://kubernetes.io/docs/concepts/policy/pod-security-policy/)
 can be used to control the security aspects of the pod deployments. This document
 walk you through an example deployment using
 [nginx unprivileged container](https://github.com/nginxinc/docker-nginx-unprivileged).

## Requirements

1. Kubernetes v1.14 or above

   * On Kubernetes versions lower than v1.14 `runAsGroup` [does not work](https://github.com/kubernetes/enhancements/issues/213)

2. Quobyte CSI deployment with PSP

3. `PodSecurityPolicy` admission plugin must be enabled.
 [comment]: <> (Edit /etc/kubernetes/manifests/kube-apiserver.yaml on master nodes and append `--enable-admission-plugins` with PodSecurityPolicy. After that restart the nodes or kube-apiserver containers)

## PSP example

Let us dive-in and create an example with unprivileged nginx container.

1. Create a namespace `psp-example` to run the example

```bash
kubectl create namespace psp-example
```

2. Create a service account `psp-user` in `psp-example` namespace

```bash
kubectl create serviceaccount -n psp-example psp-user
```

3. Create aliases for kubectl commands. `kubectl-admin` is the admin user and
 `kubectl-user` is the service account `psp-user` user for the namespace `psp-example`.

```bash
alias kubectl-admin='kubectl -n psp-example' # Admin user in the namespace "psp-example"
alias kubectl-user='kubectl --as=system:serviceaccount:psp-example:psp-user -n psp-example' # psp-user in the ns "psp-example"
```

4. Update UID and GID in [example PSP definition](example/psp/psp-example-definition.yaml) and create
 PSP.

 ```bash
 kubectl create -f example/psp/psp-example-definition.yaml
 ```

5. Create Role and RoleBindings for the `psp-user` in `psp-example` namespace

```bash
kubectl-admin create -f example/psp/psp-example-roles.yaml
```

6. Verify `psp-user` can access the `example-psp`

```bash
kubectl-user auth can-i use psp/example-psp
```

The above command should output `yes` for user to be able to deploy pods.

7. Create PVC

```bash
kubectl create -f example/pvc-dynamic-provision.yaml
```

8. Create Pod with the created PVC

```bash
kubectl-user create -f example/psp/psp-demo-nginx.yaml
```

9. Connect to pod and verify user UID/GID and volume permissions

```bash
kubectl-admin exec -it ngnix-psp-demo -- id
```

```bash
kubectl-admin exec -it ngnix-psp-demo -- ls -l /usr/share/nginx/
```

10. Copy [index file](example/psp/index.html) into the pod

```bash
kubectl cp example/index.html ngnix-dynamic-vol:/usr/share/nginx/html/
```

11. Retrive the pod ip

```bash
kubectl get pods ngnix-psp-demo -o yaml | grep podIP
```

12. Use curl to access the index page

```bash
curl http://<PodIP>:8080
```

It should retrieve Quobyte CSI welcome page (with html tags)