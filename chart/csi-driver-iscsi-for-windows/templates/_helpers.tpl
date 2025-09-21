{{/*
Create the name of the controller service account to use
*/}}
{{- define "csi-driver-iscsi-for-windows.controllerServiceAccountName" -}}
{{- if .Values.controller.serviceAccount.create }}
{{- default (printf "%s-controller" (include "csi-driver-iscsi-for-windows.fullname" .)) .Values.controller.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controller.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the node service account to use
*/}}
{{- define "csi-driver-iscsi-for-windows.nodeServiceAccountName" -}}
{{- if .Values.node.serviceAccount.create }}
{{- default (printf "%s-node" (include "csi-driver-iscsi-for-windows.fullname" .)) .Values.node.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.node.serviceAccount.name }}
{{- end }}
{{- end }}
{{/*
Expand the name of the csi-driver-iscsi-for-windows chart.
*/}}
{{- define "csi-driver-iscsi-for-windows.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name for csi-driver-iscsi-for-windows.
*/}}
{{- define "csi-driver-iscsi-for-windows.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "csi-driver-iscsi-for-windows.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels for csi-driver-iscsi-for-windows
*/}}
{{- define "csi-driver-iscsi-for-windows.labels" -}}
helm.sh/chart: {{ include "csi-driver-iscsi-for-windows.chart" . }}
{{ include "csi-driver-iscsi-for-windows.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels for csi-driver-iscsi-for-windows
*/}}
{{- define "csi-driver-iscsi-for-windows.selectorLabels" -}}
app.kubernetes.io/name: {{ include "csi-driver-iscsi-for-windows.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use for csi-driver-iscsi-for-windows
*/}}
{{- define "csi-driver-iscsi-for-windows.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "csi-driver-iscsi-for-windows.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
