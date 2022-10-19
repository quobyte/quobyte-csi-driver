{{- define "csi.sidecar.attacherContainer" }}
- name: csi-attacher
{{- if .Values.resources }}
{{- if .Values.resources.attacherContainer }}
  resources: 
{{ toYaml .Values.resources.attacherContainer | indent 4 }}
{{- end }}
{{- end }}
  image: {{ .Values.quobyte.dev.k8sAttacherImage }}
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
