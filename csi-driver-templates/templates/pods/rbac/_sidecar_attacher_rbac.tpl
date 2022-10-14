{{- define "quobyte-csi.controller.sidecarAttacherRbac" }}
---
# Attacher must be able to work with PVs, nodes and VolumeAttachments
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: quobyte-csi-attacher-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "update"]
  {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "watch", "list", "delete", "update", "create"]
  {{- end }}
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["csi.storage.k8s.io"]
    resources: ["csinodeinfos"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
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
  name: quobyte-csi-attacher-binding-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
subjects:
  - kind: ServiceAccount
    name: quobyte-csi-controller-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    namespace: kube-system 
roleRef:
  kind: ClusterRole
  name: quobyte-csi-attacher-role-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  apiGroup: rbac.authorization.k8s.io
---
{{- end }}
