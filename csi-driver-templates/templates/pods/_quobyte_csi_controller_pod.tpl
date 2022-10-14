{{- define "quobyte-csi.controllerPod" }}
---
{{- include "quobyte-csi.controller.serviceAccount" . }}
---
{{- include "quobyte-csi.controller.sidecarProvisionerRbac" . }}
---
{{- include "quobyte-csi.controller.sidecarAttacherRbac" . }}
---
{{- include "quobyte-csi.controller.sidecarSnapshotterRbac" . }}
---
{{- include "quobyte-csi.controller.sidecarResizerRbac" . }}
---
kind: StatefulSet
apiVersion: apps/v1
metadata:
  name: quobyte-csi-controller-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: quobyte-csi-controller-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  serviceName: quobyte-csi-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }} 
  replicas: {{ .Values.quobyte.csiControllerReplicas }}
  template:
    metadata:
      labels:
        app: quobyte-csi-controller-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
        role: quobyte-csi-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    spec:
      priorityClassName: system-cluster-critical
      serviceAccount: quobyte-csi-controller-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
{{- if .Values.quobyte.tolerations }}
      tolerations: 
{{ toYaml .Values.quobyte.tolerations | indent 8 }}
{{- end }}
      containers:
        {{- include "csi.sidecar.provisionerContainer" . | indent 8 }}
        {{- include "csi.sidecar.resizerContainer" . | indent 8 }}
        {{- include "csi.sidecar.attacherContainer" . | indent 8 }}
      {{- if .Values.quobyte.enableSnapshots }}        
        {{- include "csi.sidecar.snapshotterContainer" . | indent 8 }}
      {{- end }}
        {{- include "quobyte-csi.controllerContainer" . | indent 8 }}
      {{- include "quobyte-csi.controllerPodVolumeAttachments" . | indent 6 }}
---
{{- end }}
