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
    - "--leader-election=false"
{{- end }}
