{{- define "quobyte-csi.controller.sidecarProvisionerRbac" }}
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: quobyte-csi-provisioner-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
  {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  {{- end }}
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshots"]
    verbs: ["get", "list"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["get", "list"]
  {{- if .Values.quobyte.podSecurityPolicies }} 
  - apiGroups: ['policy']
    resources: ['podsecuritypolicies']
    verbs:     ['use']
    resourceNames:
    - quobyte-psp-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  {{- end }}
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: quobyte-csi-provisioner-binding-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
subjects:
  - kind: ServiceAccount
    name: quobyte-csi-controller-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: quobyte-csi-provisioner-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  apiGroup: rbac.authorization.k8s.io
---
{{- end }}
