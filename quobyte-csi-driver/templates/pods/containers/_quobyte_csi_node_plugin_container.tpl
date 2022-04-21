{{- define "quobyte-csi.nodePluginContainer" }}
- name: quobyte-csi-plugin
{{- if .Values.resources }}
{{- if .Values.resources.nodePluginContainer }}
  resources: 
{{ toYaml .Values.resources.nodePluginContainer | indent 4 }}
{{- end }}
{{- end }}
  securityContext:
    privileged: true
    capabilities:
      add: ["SYS_ADMIN"]
    allowPrivilegeEscalation: true
  image: {{ .Values.quobyte.dev.csiImage }}
  imagePullPolicy: "IfNotPresent"
  args :
    - "--csi_socket=$(CSI_ENDPOINT)"
    - "--quobyte_mount_path=$(QUOBYTE_MOUNT_PATH)"
    - "--node_name=$(KUBE_NODE_NAME)"
    - "--api_url=$(QUOBYTE_API_URL)"
    - "--driver_name={{ .Values.quobyte.csiProvisionerName }}"
    - "--driver_version={{ .Values.quobyte.dev.csiProvisionerVersion }}" 
    - "--enable_access_key_mounts={{ .Values.quobyte.enableAccessKeyMounts }}"
    - "--quobyte_version={{ .Values.quobyte.version }}"
    - "--immediate_erase={{ .Values.quobyte.immediateErase }}"
    - "--use_k8s_namespace_as_tenant={{ .Values.quobyte.useK8SNamespaceAsTenant }}" 
  env:
    - name: NODE_ID
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: CSI_ENDPOINT
      value: unix:///csi/csi.sock
    - name: QUOBYTE_MOUNT_PATH
      value:  {{ .Values.quobyte.clientMountPoint }}/mounts
    - name: KUBE_NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: QUOBYTE_API_URL
      value: {{ .Values.quobyte.apiURL }}
  volumeMounts:
    - name: kubelet-dir
      mountPath: /var/lib/kubelet/pods
      mountPropagation: "Bidirectional"
    - name: quobyte-mounts
      mountPath: {{ .Values.quobyte.clientMountPoint }}
      mountPropagation: "Bidirectional"
    - name: plugin-dir
      mountPath: /csi
    - name: log-dir
      mountPath: /tmp
    {{- if .Values.quobyte.mapHostCertsIntoContainer }}
    - name: certs
      mountPath: /etc/ssl/certs/
    {{- end }}
{{- end}}
