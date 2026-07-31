{{- define "chidrixx-controlplane.fullname" -}}
{{ .Release.Name }}-controlplane
{{- end -}}

{{- define "chidrixx-controlplane.labels" -}}
app.kubernetes.io/name: chidrixx-controlplane
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- /*
Resolves to the Secret name actually backing CHIDRIXX_AUTH_TOKEN: the
user-supplied auth.tokenSecretName if set, else the chart-managed
generated secret if auth.generate is true, else empty (unauthenticated).
*/ -}}
{{- define "chidrixx-controlplane.authSecretName" -}}
{{- if .Values.auth.tokenSecretName -}}
{{ .Values.auth.tokenSecretName }}
{{- else if .Values.auth.generate -}}
{{ include "chidrixx-controlplane.fullname" . }}-auth-token
{{- end -}}
{{- end -}}
