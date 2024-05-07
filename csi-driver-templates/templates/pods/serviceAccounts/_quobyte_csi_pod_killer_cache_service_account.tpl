{{- define "quobyte-csi-driver.podKiller.cacheServiceAccount"}}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: quobyte-csi-pod-killer-cache-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
---
{{- end}}
