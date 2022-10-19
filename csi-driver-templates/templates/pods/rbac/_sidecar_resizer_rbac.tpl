{{- define "quobyte-csi-driver.controller.sidecarResizerRbac" }}
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: external-resizer-runner-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  {{- end }}
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "patch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims/status"]
    verbs: ["patch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list", "watch", "create", "update", "patch"]
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
  name: csi-resizer-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
subjects:
  - kind: ServiceAccount
    name: quobyte-csi-controller-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: external-resizer-runner-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  apiGroup: rbac.authorization.k8s.io
---
{{- end }}
