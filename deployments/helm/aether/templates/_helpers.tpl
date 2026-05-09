{{/*
Common helpers for Aether's Helm chart.
Released names are scoped under the release name; per-component names
are derived from the release name + a stable suffix.
*/}}

{{- define "aether.fullname" -}}
{{- printf "%s-%s" .Release.Name "aether" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "aether.componentName" -}}
{{- $name := index . 1 -}}
{{- $ctx := index . 0 -}}
{{- printf "%s-%s" $ctx.Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "aether.labels" -}}
app.kubernetes.io/name: aether
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "aether.componentLabels" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{ include "aether.labels" $ctx }}
app.kubernetes.io/component: {{ $component }}
{{- end -}}

{{- define "aether.serviceAccountName" -}}
{{- if .Values.serviceAccount.name -}}
{{ .Values.serviceAccount.name }}
{{- else -}}
{{ include "aether.fullname" . }}
{{- end -}}
{{- end -}}

{{/*
Build the Postgres connection URL. If postgres.enabled, build it from
parts; otherwise use the verbatim postgresUrl value the operator
supplied.
*/}}
{{- define "aether.postgresUrl" -}}
{{- if .Values.postgres.enabled -}}
postgres://{{ .Values.postgres.user }}:$(POSTGRES_PASSWORD)@{{ include "aether.fullname" . }}-postgres:5432/{{ .Values.postgres.database }}?sslmode=disable
{{- else -}}
{{ .Values.postgresUrl }}
{{- end -}}
{{- end -}}

{{/*
Image reference for an Aether service. Prefers per-component .image.tag
then falls back to .Values.global.imageTag.
*/}}
{{- define "aether.image" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- $tag := default $ctx.Values.global.imageTag $component.image.tag -}}
{{- $registry := $ctx.Values.global.imageRegistry -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $component.image.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $component.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{/*
Common pod metadata: prometheus annotations + version labels.
*/}}
{{- define "aether.podAnnotations" -}}
{{- if .Values.metrics.prometheusAnnotations -}}
prometheus.io/scrape: "true"
prometheus.io/port: "{{ .port }}"
{{- end -}}
{{- end -}}
