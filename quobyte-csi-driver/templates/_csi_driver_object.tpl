{{- /* Define CSIDriver object with sub-feature flags */}}
{{- define "quobyte-csi.CSIDriverObject" }}
---
{{- if semverCompare ">=1.19.0" .Values.k8sVersion }}
apiVersion: storage.k8s.io/v1
{{- else }}
apiVersion: storage.k8s.io/v1beta1  
{{- end }}
kind: CSIDriver
metadata:
  name: {{ .Values.quobyte.csiProvisionerName }}
spec:
  attachRequired: false
  podInfoOnMount: false
  fsGroupPolicy: None
  requiresRepublish: false
  volumeLifecycleModes:
    - Persistent
---
{{- end }}