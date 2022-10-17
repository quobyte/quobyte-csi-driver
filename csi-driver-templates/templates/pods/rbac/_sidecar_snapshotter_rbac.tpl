{{- define "quobyte-csi-driver.controller.sidecarSnapshotterRbac" }}
  {{- if .Values.quobyte.enableSnapshots }}
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: external-snapshotter-runner-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
rules:
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
  {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  {{- end }}
  # Secret permission is optional.
  # Enable it if your driver needs secret.
  # For example, `csi.storage.k8s.io/snapshotter-secret-name` is set in VolumeSnapshotClass.
  # See https://kubernetes-csi.github.io/docs/secrets-and-credentials.html for more details.
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents"]
    verbs: ["create", "get", "list", "watch", "update", "delete"]
  - apiGroups: ["snapshot.storage.k8s.io"]
    resources: ["volumesnapshotcontents/status"]
    verbs: ["update"]
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
  name: csi-snapshotter-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
subjects:
  - kind: ServiceAccount
    name: quobyte-csi-controller-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: external-snapshotter-runner-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  apiGroup: rbac.authorization.k8s.io
---
  {{- end }}
{{- end }}
