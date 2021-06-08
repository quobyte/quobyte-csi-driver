{{- define "quobyte-csi.controllerPodVolumeAttachments" }}
volumes:
  - name: socket-dir
    emptyDir: {}
  - name: log-dir
    hostPath:
      path: /tmp
      type: Directory
  {{- if .Values.quobyte.mapHostCertsIntoContainer }}
  - name: certs
    hostPath:
      path: /etc/ssl/certs/
      type: Directory
  {{- end }}
{{- end }}
