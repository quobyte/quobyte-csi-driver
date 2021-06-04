{{- define "csi.sidecar.snapshotterContainer" }}
- name: csi-snapshotter
  {{- if .Values.resources }}
  resources: 
    {{ toYaml .Values.resources | indent 4 }}
  {{- end }}
  image: {{ .Values.quobyte.dev.k8sSnapshotterImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--v=3"
    - "--leader-election=false"
{{- end }}