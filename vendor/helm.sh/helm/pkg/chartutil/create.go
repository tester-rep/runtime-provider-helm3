/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package chartutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"helm.sh/helm/pkg/chart"
	"helm.sh/helm/pkg/chart/loader"
)

const (
	// ChartfileName is the default Chart file name.
	ChartfileName = "Chart.yaml"
	// ValuesfileName is the default values file name.
	ValuesfileName = "values.yaml"
	// TemplatesDir is the relative directory name for templates.
	TemplatesDir = "templates"
	// ChartsDir is the relative directory name for charts dependencies.
	ChartsDir = "charts"
	// IgnorefileName is the name of the Helm ignore file.
	IgnorefileName = ".helmignore"
	// IngressFileName is the name of the example ingress file.
	IngressFileName = TemplatesDir + sep + "ingress.yaml"
	// DeploymentName is the name of the example deployment file.
	DeploymentName = TemplatesDir + sep + "deployment.yaml"
	// ServiceName is the name of the example service file.
	ServiceName = TemplatesDir + sep + "service.yaml"
	// NotesName is the name of the example NOTES.txt file.
	NotesName = TemplatesDir + sep + "NOTES.txt"
	// HelpersName is the name of the example NOTES.txt file.
	HelpersName = TemplatesDir + sep + "_helpers.tpl"
)

const sep = string(filepath.Separator)

const defaultChartfile = `apiVersion: v2
name: %s
description: A Helm chart for Kubernetes

# A chart can be either an 'application' or a 'library' chart.
#
# Application charts are a collection of templates that can be packaged into versioned archives
# to be deployed.
#
# Library charts provide useful utilities or functions for the chart developer. They're included as
# a dependency of application charts to inject those utilities and functions into the rendering
# pipeline. Library charts do not define any templates and therefore cannot be deployed.
type: application

# This is the chart version. This version number should be incremented each time you make changes
# to the chart and its templates, including the app version.
version: 0.1.0

# This is the version number of the application being deployed. This version number should be
# incremented each time you make changes to the application.
appVersion: 1.16.0
`

const defaultValues = `# Default values for %s.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

replicaCount: 1

image:
  repository: nginx
  pullPolicy: IfNotPresent

nameOverride: ""
fullnameOverride: ""

service:
  type: ClusterIP
  port: 80

ingress:
  enabled: false
  annotations: {}
    # kubernetes.io/ingress.class: nginx
    # kubernetes.io/tls-acme: "true"
  hosts:
    - host: chart-example.local
      paths: []

  tls: []
  #  - secretName: chart-example-tls
  #    hosts:
  #      - chart-example.local

resources: {}
  # We usually recommend not to specify default resources and to leave this as a conscious
  # choice for the user. This also increases chances charts run on environments with little
  # resources, such as Minikube. If you do want to specify resources, uncomment the following
  # lines, adjust them as necessary, and remove the curly braces after 'resources:'.
  # limits:
  #  cpu: 100m
  #  memory: 128Mi
  # requests:
  #  cpu: 100m
  #  memory: 128Mi

nodeSelector: {}

tolerations: []

affinity: {}
`

const defaultIgnore = `# Patterns to ignore when building packages.
# This supports shell glob matching, relative path matching, and
# negation (prefixed with !). Only one pattern per line.
.DS_Store
# Common VCS dirs
.git/
.gitignore
.bzr/
.bzrignore
.hg/
.hgignore
.svn/
# Common backup files
*.swp
*.bak
*.tmp
*~
# Various IDEs
.project
.idea/
*.tmproj
`

const defaultIngress = `{{- if .Values.ingress.enabled -}}
{{- $fullName := include "<CHARTNAME>.fullname" . -}}
{{- $svcPort := .Values.service.port -}}
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  name: {{ $fullName }}
  labels:
{{ include "<CHARTNAME>.labels" . | indent 4 }}
  {{- with .Values.ingress.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
{{- if .Values.ingress.tls }}
  tls:
  {{- range .Values.ingress.tls }}
    - hosts:
      {{- range .hosts }}
        - {{ . | quote }}
      {{- end }}
      secretName: {{ .secretName }}
  {{- end }}
{{- end }}
  rules:
  {{- range .Values.ingress.hosts }}
    - host: {{ .host | quote }}
      http:
        paths:
        {{- range .paths }}
          - path: {{ . }}
            backend:
              serviceName: {{ $fullName }}
              servicePort: {{ $svcPort }}
        {{- end }}
  {{- end }}
{{- end }}
`

const defaultDeployment = `apiVersion: apps/v1beta2
kind: Deployment
metadata:
  name: {{ template "<CHARTNAME>.fullname" . }}
  labels:
{{ include "<CHARTNAME>.labels" . | indent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ include "<CHARTNAME>.name" . }}
      app.kubernetes.io/instance: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ include "<CHARTNAME>.name" . }}
        app.kubernetes.io/instance: {{ .Release.Name }}
    spec:
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 80
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /
              port: http
          readinessProbe:
            httpGet:
              path: /
              port: http
          resources:
{{ toYaml .Values.resources | indent 12 }}
    {{- with .Values.nodeSelector }}
      nodeSelector:
{{ toYaml . | indent 8 }}
    {{- end }}
    {{- with .Values.affinity }}
      affinity:
{{ toYaml . | indent 8 }}
    {{- end }}
    {{- with .Values.tolerations }}
      tolerations:
{{ toYaml . | indent 8 }}
    {{- end }}
`

const defaultService = `apiVersion: v1
kind: Service
metadata:
  name: {{ template "<CHARTNAME>.fullname" . }}
  labels:
{{ include "<CHARTNAME>.labels" . | indent 4 }}
spec:
  type: {{ .Values.service.type }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
      name: http
  selector:
    app.kubernetes.io/name: {{ include "<CHARTNAME>.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
`

const defaultNotes = `1. Get the application URL by running these commands:
{{- if .Values.ingress.enabled }}
{{- range $host := .Values.ingress.hosts }}
  {{- range .paths }}
  http{{ if $.Values.ingress.tls }}s{{ end }}://{{ $host.host }}{{ . }}
  {{- end }}
{{- end }}
{{- else if contains "NodePort" .Values.service.type }}
  export NODE_PORT=$(kubectl get -o jsonpath="{.spec.ports[0].nodePort}" services {{ template "<CHARTNAME>.fullname" . }})
  export NODE_IP=$(kubectl get nodes -o jsonpath="{.items[0].status.addresses[0].address}")
  echo http://$NODE_IP:$NODE_PORT
{{- else if contains "LoadBalancer" .Values.service.type }}
     NOTE: It may take a few minutes for the LoadBalancer IP to be available.
           You can watch the status of by running 'kubectl get svc -w {{ template "<CHARTNAME>.fullname" . }}'
  export SERVICE_IP=$(kubectl get svc {{ template "<CHARTNAME>.fullname" . }} -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
  echo http://$SERVICE_IP:{{ .Values.service.port }}
{{- else if contains "ClusterIP" .Values.service.type }}
  export POD_NAME=$(kubectl get pods -l "app={{ template "<CHARTNAME>.name" . }},release={{ .Release.Name }}" -o jsonpath="{.items[0].metadata.name}")
  echo "Visit http://127.0.0.1:8080 to use your application"
  kubectl port-forward $POD_NAME 8080:80
{{- end }}
`

const defaultHelpers = `{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "<CHARTNAME>.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "<CHARTNAME>.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "<CHARTNAME>.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "<CHARTNAME>.labels" -}}
app.kubernetes.io/name: {{ include "<CHARTNAME>.name" . }}
helm.sh/chart: {{ include "<CHARTNAME>.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}
`

// CreateFrom creates a new chart, but scaffolds it from the src chart.
func CreateFrom(chartfile *chart.Metadata, dest, src string) error {
	schart, err := loader.Load(src)
	if err != nil {
		return errors.Wrapf(err, "could not load %s", src)
	}

	schart.Metadata = chartfile

	var updatedTemplates []*chart.File

	for _, template := range schart.Templates {
		newData := transform(string(template.Data), schart.Name())
		updatedTemplates = append(updatedTemplates, &chart.File{Name: template.Name, Data: newData})
	}

	schart.Templates = updatedTemplates
	b, err := yaml.Marshal(schart.Values)
	if err != nil {
		return errors.Wrap(err, "reading values file")
	}

	var m map[string]interface{}
	if err := yaml.Unmarshal(transform(string(b), schart.Name()), &m); err != nil {
		return errors.Wrap(err, "transforming values file")
	}
	schart.Values = m

	return SaveDir(schart, dest)
}

// Create creates a new chart in a directory.
//
// Inside of dir, this will create a directory based on the name of
// chartfile.Name. It will then write the Chart.yaml into this directory and
// create the (empty) appropriate directories.
//
// The returned string will point to the newly created directory. It will be
// an absolute path, even if the provided base directory was relative.
//
// If dir does not exist, this will return an error.
// If Chart.yaml or any directories cannot be created, this will return an
// error. In such a case, this will attempt to clean up by removing the
// new chart directory.
func Create(name, dir string) (string, error) {
	path, err := filepath.Abs(dir)
	if err != nil {
		return path, err
	}

	if fi, err := os.Stat(path); err != nil {
		return path, err
	} else if !fi.IsDir() {
		return path, errors.Errorf("no such directory %s", path)
	}

	cdir := filepath.Join(path, name)
	if fi, err := os.Stat(cdir); err == nil && !fi.IsDir() {
		return cdir, errors.Errorf("file %s already exists and is not a directory", cdir)
	}

	files := []struct {
		path    string
		content []byte
	}{
		{
			// Chart.yaml
			path:    filepath.Join(cdir, ChartfileName),
			content: []byte(fmt.Sprintf(defaultChartfile, name)),
		},
		{
			// values.yaml
			path:    filepath.Join(cdir, ValuesfileName),
			content: []byte(fmt.Sprintf(defaultValues, name)),
		},
		{
			// .helmignore
			path:    filepath.Join(cdir, IgnorefileName),
			content: []byte(defaultIgnore),
		},
		{
			// ingress.yaml
			path:    filepath.Join(cdir, IngressFileName),
			content: transform(defaultIngress, name),
		},
		{
			// deployment.yaml
			path:    filepath.Join(cdir, DeploymentName),
			content: transform(defaultDeployment, name),
		},
		{
			// service.yaml
			path:    filepath.Join(cdir, ServiceName),
			content: transform(defaultService, name),
		},
		{
			// NOTES.txt
			path:    filepath.Join(cdir, NotesName),
			content: transform(defaultNotes, name),
		},
		{
			// _helpers.tpl
			path:    filepath.Join(cdir, HelpersName),
			content: transform(defaultHelpers, name),
		},
	}

	for _, file := range files {
		if _, err := os.Stat(file.path); err == nil {
			// File exists and is okay. Skip it.
			continue
		}
		if err := writeFile(file.path, file.content); err != nil {
			return cdir, err
		}
	}
	return cdir, nil
}

// transform performs a string replacement of the specified source for
// a given key with the replacement string
func transform(src, replacement string) []byte {
	return []byte(strings.ReplaceAll(src, "<CHARTNAME>", replacement))
}

func writeFile(name string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(name, content, 0644)
}
