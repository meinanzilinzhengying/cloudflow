{{/*
P23: CloudFlow Helm Chart 辅助模板
定义通用命名和标签函数，避免重复
*/}}
{{- define "cloudflow.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}
{{- define "cloudflow.namespace" -}}
{{- default .Values.global.namespace .Release.Namespace }}
{{- end }}
{{- define "cloudflow.labels" -}}
helm.sh/chart: {{ include "cloudflow.chart" . }}
{{ include "cloudflow.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
{{- define "cloudflow.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cloudflow.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
{{- define "cloudflow.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.serviceAccountName" -}}
{{- default "cloudflow-admin" .Values.serviceAccount.name }}
{{- end }}
{{- define "cloudflow.image" -}}
{{- $registry := .global.imageRegistry | default .image.registry | default "" }}
{{- $repository := .image.repository }}
{{- $tag := .image.tag | default "latest" }}
{{- if $registry }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- else }}
{{- printf "%s:%s" $repository $tag }}
{{- end }}
{{- end }}
{{- define "cloudflow.center.name" -}}
{{- printf "%s-center" (include "cloudflow.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.edge.name" -}}
{{- printf "%s-edge" (include "cloudflow.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.agent.name" -}}
{{- printf "%s-agent" (include "cloudflow.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.frontend.name" -}}
{{- printf "%s-frontend" (include "cloudflow.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.config.name" -}}
{{- printf "%s-config" (include "cloudflow.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "cloudflow.secret.name" -}}
{{- printf "%s-secrets" (include "cloudflow.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
