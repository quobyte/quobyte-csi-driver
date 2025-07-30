{{- define "quobyte-csi-driver.podKiller.cachePod" }}
{{- if .Values.quobyte.podKiller.enable }}
---
{{- include "quobyte-csi-driver.podKiller.cacheServiceAccount" . }}
---
{{- include "quobyte-csi-driver.podKiller.cacheRbac" . }}
---
kind: Deployment
apiVersion: apps/v1
metadata:
  name: quobyte-csi-pod-killer-cache-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: quobyte-csi-pod-killer-cache-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  replicas: {{ .Values.quobyte.podKiller.cacheReplicas }}
  template:
    metadata:
      labels:
        app: quobyte-csi-pod-killer-cache-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
        role: quobyte-csi-pod-killer-cache-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
    spec:
    {{- if .Values.quobyte.podKiller.nodeSelector }}
      nodeSelector:
        {{- .Values.quobyte.podKiller.nodeSelector | toYaml | nindent 8 }}
    {{- end }}
      priorityClassName: system-cluster-critical
      serviceAccount: quobyte-csi-pod-killer-cache-sa-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
{{- if .Values.quobyte.tolerations }}
      tolerations: 
{{ toYaml .Values.quobyte.tolerations | indent 8 }}
{{- end }}
      containers:
        {{- include "quobyte-csi-driver.podKiller.cacheContainer" . | indent 8 }}
---
apiVersion: v1
kind: Service
metadata:
  name: quobyte-pod-killer-cache-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  namespace: kube-system
spec:
  selector:
    app: quobyte-csi-pod-killer-cache-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
---
{{- end }}
{{- end }}
