{{- define "quobyte-csi.controllerContainer" }}
- name: quobyte-csi-plugin
  {{- if .Values.resources }}
  resources: 
    {{ toYaml .Values.resources | indent 4 }}
  {{- end }}
  image: {{ .Values.quobyte.dev.csiImage }}
  imagePullPolicy: "IfNotPresent"
  args :
    - "--csi_socket=$(CSI_ENDPOINT)"
    - "--quobyte_mount_path=$(QUOBYTE_MOUNT_PATH)"
    - "--node_name=$(KUBE_NODE_NAME)"
    - "--api_url=$(QUOBYTE_API_URL)" 
    - "--driver_name={{ .Values.quobyte.csiProvisionerName }}"
    - "--driver_version={{ .Values.quobyte.dev.csiProvisionerVersion }}"
    - "--enable_access_keys={{ .Values.quobyte.enableAccessKeys }}"
    - "--use_k8s_namespace_as_tenant={{ .Values.quobyte.useK8SNamespaceAsTenant }}" 
  env:
    - name: NODE_ID
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: CSI_ENDPOINT
      value: unix:///var/lib/csi/sockets/pluginproxy/csi.sock
    - name: QUOBYTE_MOUNT_PATH
      value:  {{ .Values.quobyte.clientMountPoint }}/mounts
    - name: KUBE_NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: QUOBYTE_API_URL
      value: {{ .Values.quobyte.apiURL }} 
  volumeMounts:
    - name: socket-dir
      mountPath: /var/lib/csi/sockets/pluginproxy/
    - name: log-dir
      mountPath: /tmp
    {{- if .Values.quobyte.mapHostCertsIntoContainer }}
    - name: certs
      mountPath: /etc/ssl/certs/
    {{- end }}
{{- end }}
