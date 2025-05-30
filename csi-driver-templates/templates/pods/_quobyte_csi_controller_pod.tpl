{{- define "quobyte-csi-driver.controllerPod" }}
---
{{- include "quobyte-csi-driver.controller.serviceAccount" . }}
---
{{- include "quobyte-csi-driver.controller.sidecarProvisionerRbac" . }}
---
{{- include "quobyte-csi-driver.controller.sidecarAttacherRbac" . }}
---
{{- include "quobyte-csi-driver.controller.sidecarSnapshotterRbac" . }}
---
{{- include "quobyte-csi-driver.controller.sidecarResizerRbac" . }}
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
  replicas: {{ .Values.quobyte.csiControllerReplicas }}
  template:
    metadata:
      labels:
        app: quobyte-csi-controller-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
        role: quobyte-csi-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    spec:
    {{- if default "" .Values.quobyte.nodeSelector | trim }}
      nodeSelector:
        {{ .Values.quobyte.nodeSelector | trim }}
    {{- end }}
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
        {{- include "quobyte-csi-driver.controllerContainer" . | indent 8 }}
      {{- include "quobyte-csi-driver.controllerPodVolumeAttachments" . | indent 6 }}
---
{{- end }}
