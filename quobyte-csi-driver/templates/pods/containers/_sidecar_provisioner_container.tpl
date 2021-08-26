{{- define "csi.sidecar.provisionerContainer" }}
- name: csi-provisioner
{{- if .Values.resources }}
  resources: 
{{ toYaml .Values.resources | indent 4 }}
{{- end }}
  image: {{ .Values.quobyte.dev.k8sProvisionerImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--csi-address=$(ADDRESS)"
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
