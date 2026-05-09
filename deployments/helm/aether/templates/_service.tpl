{{/*
aether.serviceManifests renders Deployment + Service for one Aether
backend component. Args (positional):
  0: $ (chart context)
  1: component values (e.g. .Values.audit)
  2: component name (used in labels and Service/Deployment metadata)
  3: list of additional env vars (rendered as YAML); e.g. for AETHER_PG_URL
  4: extra container args (list)
*/}}
{{- define "aether.backendDeploy" -}}
{{- $ctx := index . 0 -}}
{{- $cfg := index . 1 -}}
{{- $component := index . 2 -}}
{{- $envExtra := index . 3 -}}
{{- $argsExtra := index . 4 -}}
{{- $name := printf "%s-%s" (include "aether.fullname" $ctx) $component -}}
{{- if $cfg.enabled }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $name }}
  labels: {{- include "aether.componentLabels" (list $ctx $component) | nindent 4 }}
spec:
  replicas: {{ $cfg.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: aether
      app.kubernetes.io/instance: {{ $ctx.Release.Name }}
      app.kubernetes.io/component: {{ $component }}
  template:
    metadata:
      labels: {{- include "aether.componentLabels" (list $ctx $component) | nindent 8 }}
      annotations:
        {{- if $ctx.Values.metrics.prometheusAnnotations }}
        prometheus.io/scrape: "true"
        prometheus.io/port: "{{ $cfg.service.port }}"
        {{- end }}
    spec:
      serviceAccountName: {{ include "aether.serviceAccountName" $ctx }}
      securityContext:
        {{- toYaml $ctx.Values.podSecurityContext | nindent 8 }}
      containers:
        - name: {{ $component }}
          image: {{ include "aether.image" (list $ctx $cfg) | quote }}
          imagePullPolicy: {{ $ctx.Values.global.imagePullPolicy }}
          {{- with $argsExtra }}
          args:
            {{- range . }}
            - {{ . | quote }}
            {{- end }}
          {{- end }}
          env:
            {{- if $ctx.Values.postgres.enabled }}
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: {{ if $ctx.Values.postgres.existingSecret }}{{ $ctx.Values.postgres.existingSecret }}{{ else }}{{ printf "%s-postgres" (include "aether.fullname" $ctx) }}{{ end }}
                  key: password
            {{- end }}
            {{- with $envExtra }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          ports:
            - name: http
              containerPort: {{ $cfg.service.port }}
          readinessProbe:
            httpGet:
              path: /v1/health
              port: http
            initialDelaySeconds: 2
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /v1/health
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
          resources:
            {{- toYaml $cfg.resources | nindent 12 }}
          securityContext:
            {{- toYaml $ctx.Values.containerSecurityContext | nindent 12 }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $name }}
  labels: {{- include "aether.componentLabels" (list $ctx $component) | nindent 4 }}
spec:
  type: ClusterIP
  ports:
    - name: http
      port: {{ $cfg.service.port }}
      targetPort: http
  selector:
    app.kubernetes.io/name: aether
    app.kubernetes.io/instance: {{ $ctx.Release.Name }}
    app.kubernetes.io/component: {{ $component }}
{{- end -}}
{{- end -}}
