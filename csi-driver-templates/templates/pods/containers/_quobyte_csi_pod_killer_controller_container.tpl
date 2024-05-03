{{- define "quobyte-csi-driver.podKiller.cacheContainer" }}
- name: quobyte-csi-pod-killer-cache
{{- if .Values.resources }}
{{- if .Values.resources.podKillerCache }}
  resources: 
{{ toYaml .Values.resources.podKillerCache | indent 4 }}
{{- end }}
{{- end }}
  image: {{ .Values.quobyte.dev.podKillerImage }}
  ports:
    - containerPort: 8080
  imagePullPolicy: "IfNotPresent"
  args:
    - "--driver_name={{ .Values.quobyte.csiProvisionerName }}"
    - "--role=cache"
{{- end}}
