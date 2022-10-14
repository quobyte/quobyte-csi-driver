{{- define "csi.sidecar.snapshotterContainer" }}
- name: csi-snapshotter
{{- if .Values.resources }}
{{- if .Values.resources.snapshotterContainer }}
  resources: 
{{ toYaml .Values.resources.snapshotterContainer | indent 4 }}
{{- end }}
{{- end }}
  image: {{ .Values.quobyte.dev.k8sSnapshotterImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--v=3"
    {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
    - "--leader-election=true"
    {{- end }}
    - "--csi-address=$(ADDRESS)"
  env:
    - name: ADDRESS
      value: /var/lib/csi/sockets/pluginproxy/csi.sock
  volumeMounts:
    - name: socket-dir
      mountPath: /var/lib/csi/sockets/pluginproxy/
{{- end }}
