{{- define "quobyte-csi-driver.nodeDriverPod" }}
---
{{- include "quobyte-csi-driver.nodeDriver.serviceAccount" . }}
---
{{- include "quobyte-csi-driver.nodeDriver.sidecarDriverRegistrarRbac" . }}
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
        {{- include "quobyte-csi-driver.nodeDriverContainer" . | indent 8 }}
        {{- include "quobyte-csi-driver.podKillerContainer" . | indent 8 }}
      {{- include "quobyte-csi-driver.nodeDriverPodVolumeAttachments" . | indent 6 }}
---
{{- end }}
