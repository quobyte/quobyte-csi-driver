{{- define "quobyte-csi-driver.nodeDriver.sidecarDriverRegistrarRbac" }}
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: quobyte-csi-driver-registrar-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
rules:
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  {{- if .Values.quobyte.podKiller.enable }}
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["list", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["list"]
  {{- end }}
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
  name: quobyte-csi-driver-registrar-binding-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
subjects:
  - kind: ServiceAccount
    name: quobyte-csi-node-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: quobyte-csi-driver-registrar-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  apiGroup: rbac.authorization.k8s.io
---
{{- end }}
