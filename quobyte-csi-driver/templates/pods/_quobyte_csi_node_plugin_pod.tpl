{{- define "quobyte-csi.nodePluginPod" }}
---
{{- include "quobyte-csi.nodePlugin.serviceAccount" . }}
---
{{- include "quobyte-csi.nodePlugin.sidecarDriverRegistrarRbac" . }}
---
kind: DaemonSet
apiVersion: apps/v1
metadata:
  name: quobyte-csi-node-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: quobyte-csi-node-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  template:
    metadata:
      labels:
        app: quobyte-csi-node-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
        role: quobyte-csi
    spec:
      priorityClassName: system-node-critical
      serviceAccount: quobyte-csi-node-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
      hostNetwork: true
{{- if .Values.quobyte.tolerations }}
      tolerations: 
{{ toYaml .Values.quobyte.tolerations | indent 8 }}
{{- end }}
      containers:
        {{- include "csi.sidecar.nodeRegistrarContainer" . | indent 8 }}
        {{- include "quobyte-csi.nodePluginContainer" . | indent 8 }}
        {{- include "quobyte-csi.podKillerContainer" . | indent 8 }}
      {{- include "quobyte-csi.nodePluginPodVolumeAttachments" . | indent 6 }}
---
{{- end }}
