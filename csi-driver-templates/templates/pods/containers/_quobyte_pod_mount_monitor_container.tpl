{{- define "quobyte-csi-driver.podKiller.mountMonitor" }}
{{- if .Values.quobyte.podKiller.enable }}
- name: quobyte-csi-mount-monitor
{{- if .Values.resources }}
{{- if .Values.resources.podKillerContainer }}
  resources: 
{{ toYaml .Values.resources.podKillerContainer | indent 4 }}
{{- end }}
{{- end }}
  securityContext:
    privileged: true
  image: {{ .Values.quobyte.dev.podKillerImage }}
  imagePullPolicy: "IfNotPresent"
  args:
    - "--node_name=$(KUBE_NODE_NAME)"
    - "--driver_name={{ .Values.quobyte.csiProvisionerName }}"
    - "--service_url=http://quobyte-pod-killer-cache.$(NAMESPACE).svc.cluster.local:80/"
    - "--monitoring_interval={{ .Values.quobyte.podKiller.monitoringInterval }}"
    - "--role=monitor"
  env:
    - name: NAMESPACE
      valueFrom:
        fieldRef:
          fieldPath: metadata.namespace
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
