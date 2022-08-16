{{- define "csi.sidecar.provisionerContainer" }}
- name: csi-provisioner
{{- if .Values.resources }}
{{- if .Values.resources.provisionerContainer }}
  resources: 
{{ toYaml .Values.resources.provisionerContainer | indent 4 }}
{{- end }}
{{- end }}
  image: {{ .Values.quobyte.dev.k8sProvisionerImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--csi-address=$(ADDRESS)"
    {{- if gt (.Values.quobyte.csiControllerReplicas | toString | atoi) 1 }}
    - "--leader-election=true"
    {{- end }}
    - "--v=3"
    - "--extra-create-metadata=true"
    - "--timeout=5m"
  env:
    - name: ADDRESS
      value: /var/lib/csi/sockets/pluginproxy/csi.sock
  volumeMounts:
    - name: socket-dir
      mountPath: /var/lib/csi/sockets/pluginproxy/
{{- end }}
