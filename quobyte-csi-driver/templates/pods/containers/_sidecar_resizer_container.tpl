{{- define "csi.sidecar.resizerContainer" }}
- name: csi-resizer
  {{- if .Values.resources }}
  resources: 
    {{ toYaml .Values.resources | indent 4 }}
  {{- end }}
  image: {{ .Values.quobyte.dev.k8sResizerImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--v=3"
    - "--csi-address=$(ADDRESS)"
  env:
    - name: ADDRESS
      value: /var/lib/csi/sockets/pluginproxy/csi.sock
  volumeMounts:
    - name: socket-dir
      mountPath: /var/lib/csi/sockets/pluginproxy/
{{- end }}