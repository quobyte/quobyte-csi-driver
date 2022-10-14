{{- define "csi.sidecar.resizerContainer" }}
- name: csi-resizer
{{- if .Values.resources }}
{{- if .Values.resources.resizerContainer }}
  resources: 
{{ toYaml .Values.resources.resizerContainer | indent 4 }}
{{- end }}
{{- end }}
  image: {{ .Values.quobyte.dev.k8sResizerImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--v=3"
    - "--csi-address=$(ADDRESS)"
    {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
    - "--leader-election=true"
    {{- end }}
  env:
    - name: ADDRESS
      value: /var/lib/csi/sockets/pluginproxy/csi.sock
  volumeMounts:
    - name: socket-dir
      mountPath: /var/lib/csi/sockets/pluginproxy/
{{- end }}
