{{- define "chidrixx-controlplane.fullname" -}}
{{ .Release.Name }}-controlplane
{{- end -}}

{{- define "chidrixx-controlplane.labels" -}}
app.kubernetes.io/name: chidrixx-controlplane
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- /*
Resolves to the Secret name backing CHIDRIXX_ADMIN_PASSWORD: the
user-supplied auth.adminPasswordSecretName if set, else the chart-managed
generated secret if auth.generate is true, else empty (the control plane
binary generates and logs its own random password on first boot instead).
*/ -}}
{{- define "chidrixx-controlplane.adminPasswordSecretName" -}}
{{- if .Values.auth.adminPasswordSecretName -}}
{{ .Values.auth.adminPasswordSecretName }}
{{- else if .Values.auth.generate -}}
{{ include "chidrixx-controlplane.fullname" . }}-admin-password
{{- end -}}
{{- end -}}
