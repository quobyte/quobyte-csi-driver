{{- define "csi.sidecar.nodeRegistrarContainer" }}
- name: csi-node-driver-registrar
{{- if .Values.resources }}
{{- if .Values.resources.nodeRegistrarContainer }}
  resources: 
{{ toYaml .Values.resources.nodeRegistrarContainer | indent 4 }}
{{- end }}
{{- end }}
  image: {{ .Values.quobyte.dev.k8sNodeRegistrarImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--v=3"
    - "--csi-address=$(ADDRESS)"
    - "--kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)"
  lifecycle:
    preStop:
      exec:
        command: ["/bin/sh", "-c", "rm -rf /registration/{{ .Values.quobyte.csiProvisionerName }} /registration/{{ .Values.quobyte.csiProvisionerName }}-reg.sock"]
  env:
    - name: ADDRESS
      value: /csi/csi.sock
    - name: DRIVER_REG_SOCK_PATH
      value: /var/lib/kubelet/plugins/{{ .Values.quobyte.csiProvisionerName }}/csi.sock
    - name: KUBE_NODE_NAME
      valueFrom:
        fieldRef:
            fieldPath: spec.nodeName
  volumeMounts:
    - name: plugin-dir
      mountPath: /csi/
    - name: registration-dir
      mountPath: /registration/
{{- end}}
