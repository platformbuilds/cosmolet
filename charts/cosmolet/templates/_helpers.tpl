
{{- define "cosmolet.serviceAccountName" -}}
{{- if .Values.serviceAccount.name -}}
{{ .Values.serviceAccount.name }}
{{- else -}}
{{ include "cosmolet.fullname" . }}
{{- end -}}
{{- end -}}

{{- define "cosmolet.fullname" -}}
cosmolet
{{- end -}}
