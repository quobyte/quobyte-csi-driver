{{- define "quobyte-csi-driver.nodeDriverPodVolumeAttachments" }}
volumes:
  - name: kubelet-dir
    hostPath:
      path: /var/lib/kubelet/pods
      type: Directory
  - name: quobyte-mounts
    hostPath:
      # Quobyte client also should use the same mount point
      path: {{ .Values.quobyte.clientMountPoint }}
      type: DirectoryOrCreate
  - name: plugin-dir
    hostPath:
      # required by kubernetes CSI
      path: /var/lib/kubelet/plugins/{{ .Values.quobyte.csiProvisionerName }}
      type: DirectoryOrCreate
  - name: registration-dir
    hostPath:
      path: /var/lib/kubelet/plugins_registry/
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
