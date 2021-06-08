{{- define "quobyte-csi.controller.serviceAccount" }}
---
kind: ServiceAccount
apiVersion: v1
metadata:
  name: quobyte-csi-controller-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
---
{{- end }}
