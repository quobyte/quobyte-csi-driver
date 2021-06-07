**Sample RBAC** for the sidecar containers can be found at 
https://github.com/kubernetes-csi/\<sidecar-project\>/tree/master/deploy/kubernetes

Updating a sidecar container version might require update to the RBAC permissions of the
`_sidecar_<container>_rbac.tpl`. 