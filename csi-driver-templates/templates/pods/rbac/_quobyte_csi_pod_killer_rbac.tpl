{{- define "quobyte-csi-driver.podKiller.cacheRbac" }}
{{- if .Values.quobyte.podKiller.enable }}
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: quobyte-csi-pod-killer-cache-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list", "watch"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["list", "watch"]
---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: quobyte-csi-pod-killer-cache-binding-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
subjects:
  - kind: ServiceAccount
    name: quobyte-csi-pod-killer-cache-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: quobyte-csi-pod-killer-cache-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  apiGroup: rbac.authorization.k8s.io
---
{{- end }}
{{- end }}
