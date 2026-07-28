{{- define "kharcha.fullname" -}}
{{ .Release.Name }}-kharcha
{{- end -}}

{{- define "kharcha.labels" -}}
app.kubernetes.io/name: kharcha
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
