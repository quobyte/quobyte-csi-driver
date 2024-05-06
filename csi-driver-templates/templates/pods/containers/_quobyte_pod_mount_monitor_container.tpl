{{- define "quobyte-csi-driver.podKiller.mountMonitor" }}
{{- if .Values.quobyte.podKiller.enable }}
- name: quobyte-csi-mount-monitor
{{- if .Values.resources }}
{{- if .Values.resources.podMountMonitor }}
  resources: 
{{ toYaml .Values.resources.podMountMonitor | indent 4 }}
{{- end }}
{{- end }}
  securityContext:
    privileged: true
  image: {{ .Values.quobyte.dev.podKillerImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--node_name=$(KUBE_NODE_NAME)"
    - "--driver_name={{ .Values.quobyte.csiProvisionerName }}"
    - "--service_url=http://quobyte-pod-killer-cache:80/"
    - "--monitoring_interval={{ .Values.quobyte.podKiller.monitoringInterval }}"
    - "--role=monitor"
  env:
    - name: NODE_ID
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: KUBE_NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
  volumeMounts:
    - name: kubelet-dir
      mountPath: /var/lib/kubelet/pods
      mountPropagation: "Bidirectional"
{{- end }}
{{- end }}