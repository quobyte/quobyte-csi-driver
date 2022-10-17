{{- define "quobyte-csi-driver.controllerPodVolumeAttachments" }}
volumes:
  - name: socket-dir
    emptyDir: {}
  - name: quobyte-mounts
    hostPath:
      # Quobyte client also should use the same mount point
      path: {{ .Values.quobyte.clientMountPoint }}
      type: DirectoryOrCreate
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
