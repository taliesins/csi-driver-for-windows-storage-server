{{/*
Create the name of the controller resources
*/}}
{{- define "csi-driver-for-windows-storage-server.controllerName" -}}
{{- printf "%s-controller" ((include "csi-driver-for-windows-storage-server.fullname" .) | trunc 52 | trimSuffix "-") }}
{{- end }}

{{/*
Create the name of the controller service account to use
*/}}
{{- define "csi-driver-for-windows-storage-server.controllerServiceAccountName" -}}
{{- if .Values.controller.serviceAccount.create }}
{{- default (include "csi-driver-for-windows-storage-server.controllerName" .) .Values.controller.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controller.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the node resources
*/}}
{{- define "csi-driver-for-windows-storage-server.nodeName" -}}
{{- printf "%s-node" ((include "csi-driver-for-windows-storage-server.fullname" .) | trunc 58 | trimSuffix "-") }}
{{- end }}

{{/*
Create the name of the node service account to use
*/}}
{{- define "csi-driver-for-windows-storage-server.nodeServiceAccountName" -}}
{{- if .Values.node.serviceAccount.create }}
{{- default (include "csi-driver-for-windows-storage-server.nodeName" .) .Values.node.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.node.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the WinRM credentials secret to use
*/}}
{{- define "csi-driver-for-windows-storage-server.winrmSecretName" -}}
{{- default (printf "%s-winrm" ((include "csi-driver-for-windows-storage-server.fullname" .) | trunc 57 | trimSuffix "-")) .Values.winrm.existingSecret }}
{{- end }}

{{/*
Expand the name of the csi-driver-for-windows-storage-server chart.
*/}}
{{- define "csi-driver-for-windows-storage-server.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name for csi-driver-for-windows-storage-server.
*/}}
{{- define "csi-driver-for-windows-storage-server.fullname" -}}
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
{{- define "csi-driver-for-windows-storage-server.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels for csi-driver-for-windows-storage-server
*/}}
{{- define "csi-driver-for-windows-storage-server.labels" -}}
helm.sh/chart: {{ include "csi-driver-for-windows-storage-server.chart" . }}
{{ include "csi-driver-for-windows-storage-server.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels for csi-driver-for-windows-storage-server
*/}}
{{- define "csi-driver-for-windows-storage-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "csi-driver-for-windows-storage-server.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use for csi-driver-for-windows-storage-server
*/}}
{{- define "csi-driver-for-windows-storage-server.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "csi-driver-for-windows-storage-server.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
