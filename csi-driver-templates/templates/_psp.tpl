{{- define "quobyte-csi-driver.psp" }}
---
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: quobyte-psp-{{ .Values.quobyte.csiProvisionerName | replace "." "-"  }}
  annotations:
    seccomp.security.alpha.kubernetes.io/allowedProfileNames: '*'
spec:
  privileged: true
  allowPrivilegeEscalation: true
  allowedCapabilities:
    - '*'
  volumes:
    - '*'
  hostNetwork: true
  hostPorts:
    - min: 0
      max: 65535
  runAsUser:
    rule: 'RunAsAny'
  seLinux:
    rule: 'RunAsAny'
  supplementalGroups:
    rule: 'RunAsAny'
  fsGroup:
    rule: 'RunAsAny'
---
{{- end }}
