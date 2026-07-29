{{- define "chidrixx-controlplane.fullname" -}}
{{ .Release.Name }}-controlplane
{{- end -}}

{{- define "chidrixx-controlplane.labels" -}}
app.kubernetes.io/name: chidrixx-controlplane
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
