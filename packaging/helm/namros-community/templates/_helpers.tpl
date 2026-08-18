{{- define "namros.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "namros.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "namros.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "namros.labels" -}}
app.kubernetes.io/name: {{ include "namros.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}

{{- define "namros.selectorLabels" -}}
app.kubernetes.io/name: {{ include "namros.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "namros.secretName" -}}
{{- if .Values.rootCredentials.existingSecret -}}
{{- .Values.rootCredentials.existingSecret -}}
{{- else -}}
{{- printf "%s-root" (include "namros.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "namros.tikvPDEndpoints" -}}
{{- if .Values.embedded.tikv.enabled -}}
{{- printf "%s-pd:2379" (include "namros.fullname" .) -}}
{{- else -}}
{{- required "metadata.tikv.pdEndpoints is required when embedded.tikv.enabled=false" .Values.metadata.tikv.pdEndpoints -}}
{{- end -}}
{{- end -}}

{{- define "namros.etcdEndpoints" -}}
{{- if .Values.embedded.etcd.enabled -}}
{{- printf "http://%s-etcd:2379" (include "namros.fullname" .) -}}
{{- else -}}
{{- required "coordination.etcd.endpoints is required when embedded.etcd.enabled=false" .Values.coordination.etcd.endpoints -}}
{{- end -}}
{{- end -}}

{{- define "namros.sbsServiceReadyURLs" -}}
{{- $root := . -}}
{{- range $i, $node := .Values.sbs.service.nodes -}}{{- if $i }},{{ end -}}http://{{ include "namros.fullname" $root }}-{{ $node.id }}:9081/readyz{{- end -}}
{{- end -}}

{{- define "namros.sbsDataReadyURLs" -}}
{{- $root := . -}}
{{- range $i, $node := .Values.sbs.data.nodes -}}{{- if $i }},{{ end -}}http://{{ include "namros.fullname" $root }}-{{ $node.id }}:9082/readyz{{- end -}}
{{- end -}}

{{- define "namros.sbsDataNodeIDs" -}}
{{- range $i, $node := .Values.sbs.data.nodes -}}{{- if $i }},{{ end -}}{{ $node.id }}{{- end -}}
{{- end -}}

{{- define "namros.sbsDataGRPCEndpoints" -}}
{{- $root := . -}}
{{- range $i, $node := .Values.sbs.data.nodes -}}{{- if $i }},{{ end -}}{{ include "namros.fullname" $root }}-{{ $node.id }}:9444{{- end -}}
{{- end -}}

{{- define "namros.sbsDataAdminHTTPEndpoints" -}}
{{- $root := . -}}
{{- range $i, $node := .Values.sbs.data.nodes -}}{{- if $i }},{{ end -}}http://{{ include "namros.fullname" $root }}-{{ $node.id }}:9082{{- end -}}
{{- end -}}

{{- define "namros.sbsDataZones" -}}
{{- range $i, $node := .Values.sbs.data.nodes -}}{{- if $i }},{{ end -}}{{ default "zone-a" $node.zone }}{{- end -}}
{{- end -}}

{{- define "namros.secretVolume" -}}
- name: namros-root-credentials
  secret:
    secretName: {{ include "namros.secretName" . }}
    items:
      - key: {{ .Values.rootCredentials.accessKeyKey }}
        path: namros_root_access_key_id
      - key: {{ .Values.rootCredentials.secretAccessKeyKey }}
        path: namros_root_secret_access_key
{{- end -}}

{{- define "namros.secretMount" -}}
- name: namros-root-credentials
  mountPath: /run/secrets
  readOnly: true
{{- end -}}

{{- define "namros.secretEnv" -}}
- name: NAMROS_ROOT_ACCESS_KEY_ID_FILE
  value: /run/secrets/namros_root_access_key_id
- name: NAMROS_ROOT_SECRET_ACCESS_KEY_FILE
  value: /run/secrets/namros_root_secret_access_key
{{- end -}}
