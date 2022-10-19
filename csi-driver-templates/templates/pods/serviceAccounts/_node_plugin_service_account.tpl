{{- define "quobyte-csi.nodePlugin.serviceAccount"}}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: quobyte-csi-node-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
---
{{- end}}
