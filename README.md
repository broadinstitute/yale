# yale


[![Go Report Card](https://goreportcard.com/badge/github.com/broadinstitute/yale)](https://goreportcard.com/report/github.com/broadinstitute/yale)
![latest build](https://github.com/broadinstitute/yale/actions/workflows/build.yaml/badge.svg?branch=main)

Yale is a Go service that manages Google Cloud Platform (GCP) service account (SA) keys and Azure Client Secrets used by Kubernetes resources. As stated in  GCP documents, <em>Service accounts are unique identities used to facilitate programmatic access to GCP APIs, And Azure App registrations use a similar pattern of having a client ID and client secret to achieve the same effect.</em>. For compliance, keys must be rotated at least every 90 days.

Yale has five purposes:
1. Create new secrets for new GcpSaKEy or AzureClientSecret resources and store referenced SA keys in a Secrets.
2. Detect keys that need to be rotated.
3. Update GSK generated Secrets with new keys.
4. Check for old keys and disable them.
5. Delete disabled keys.

Yale monitors GcpSaKey resouces to manage the referenced GCP SA. GcpSaKey is custom resource definition, or [CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), that tells Yale how to create or modify secrets holding GCP SA keys. These secrets are referenced in Kubernetes resources, ie. Deployments, Cronjobs, etc, and when modified, trigger rolling updates to update those resources.

See [DESIGN.md](DESIGN.md) for more information about how this functionality is implemented.


## Getting Started

A GcpSaKey or AzureClientSecret CRD needs to be created via helm for every service account that Yale will manage rotation for. For example,

See [examples/azureClientSecret.yaml](examples/azureClientSecret.yaml) for an example of a AzureClientSecret.

```
apiVersion: yale.broadinstitute.org/v1beta1
kind: GcpSaKey
metadata:
  name: example-gcpsakey
spec:
  secret:
      name: example-sa-secret
      pemKeyName: example.pem
      jsonKeyName: example.json
    keyRotation:
      rotateAfter: 90
      deleteAfter: 7
      disableAfter: 7
    googleServiceAccount:
      name: example@broad-dsde-dev.iam.gserviceaccount.com
      project: broad-dsde-dev
```
or using the [yale library](https://github.com/broadinstitute/terra-helmfile/tree/master/charts/yalelib) with customized values:
```
{{- include "libchart.gcpsakey" (list . "example.gcpsakey" ) -}}

{{- define "example.gcpsakey" -}}
spec:
  keyRotation:
    rotateAfter: 20
    deleteAfter: 5
    disableAfter: 5
  googleServiceAccount:
    name: {{ .Values.example.googleServiceAccount.name }}
  secret:
    name: {{ .Chart.Name }}-sa-secret
    pemKeyName: example-account.pem
    jsonKeyName: sqlproxy-service-account.json
{{- end }}
```
or using default values
```
{{- include "libchart.gcpsakey" (list . "example.gcpsakey" ) -}}

{{- define "example.gcpsakey" -}}
spec:
   googleServiceAccount:
      name: {{ .Values.example.googleServiceAccount.name }}
   secret:
      name: {{ .Chart.Name }}-sa-secret
{{- end }}
```

Where:

| Field | Type | Required| Default | Description |
|-----|------|------|---------|-------------|
| metadata.name| string| yes | | Name of Resource. **Name must end in gcpsakey**|
| spec.secret.name | string | yes|  | Name of Secret that houses SA. **Name must end in "sa-secret"** |
|spec.secret.pemKeyName | string |  no | service-account.pem | Name of Secret data field that stores pem private key|
| spec.secret.jsonKeyName | string | no | service-account.json | Name of Secret data field that stores private key |
| spec.keyRotation.rotateAfter | int | no | 65 | Amount of days before key is rotated |
| spec.keyRotation.deleteAfter | int | no | 15 | Amount of days key is disabled before deleting |
| spec.keyRotation.disableAfter | int | no | 10 | Amount of days since key was last authenticated against before disabling |
| spec.googleServiceAccount.name | string | yes |  | Email of the GCP SA |
| spec.googleServiceAccount.project | string | yes |  | Google project ID SA is associated with|

The default values are not required if using the Yale library, otherwise they must be included in the chart. When using the Yale library make sure to add the library as a dependency in the [Chart.yaml](https://github.com/broadinstitute/terra-helmfile/blob/4db9e59714ed74ec9c61e66f6af610c92f04f073/charts/agora/Chart.yaml#L26) file and here's an example [value.yaml](https://github.com/broadinstitute/terra-helmfile/blob/e8068635cb164a9df5aa2820451144aa2fcee044/charts/agora/values.yaml#L114) file. Read more about helm libraries [here](https://helm.sh/docs/topics/library_charts/).

That's all! Yale takes care of the rest!

## Installation

Yale is deployed by the DSP-DevOps team in every cluster we manage, if you are a Terra application developer looking 


Yale is intended to be deployed as a kubernetes cronjob. Since keys rotate at the project level, not service, the cronJob performs all functions and checks against SA keys defined in GcpSaKey resource. Therefore, the cronJob does not need to be added to each service. The cronjob will run Yale every 2 minutes.

Yale is intended to be installed via helm, the chart can be found in the [terra-helmfile](https://github.com/broadinstitute/terra-helmfile/tree/master/charts/yale) repo.

When deployed as a cronjob [via helm](https://github.com/broadinstitute/terra-helmfile/blob/master/charts/yale/templates/cronJob.yam), Yale uses in cluster authentication provided by kubernetes/client-go. There are no additional steps required to configure this.

Yale also requires a GCP service account with roles/iam.serviceAccountKeyAdmin role. The cronjob expects service account credentials to be mounted to the pod or workload identity can be used instead.

## Developer Quick Start

This repo features a set of utility scripts in /scripts to help new contributors get set up with the project.

Here is a quick workflow to get a basic local environment up and running
 1. Run `./scripts/setup` It will instruct you to install any necessary tools that need to be on your path
 2. Run `./scripts/run tests` will execute the test suite on your local machine
 3. Run `./scripts/run local` To trigger a run of the yale daemon aganist a local k8s cluster created in the setup step. It is expected for the process to run to completion and then exit.

See [CONTRIBUTING.md](CONTRIBUTING.md) for more information on Developing Yale.
See [DESIGN.md](DESIGN.md) for more information on the layout or the repo and design of the application

### Runtime flags

These flags are only useful when running
```
Usage of yale:
  -kubeconfig string
    	(optional) absolute path to kubectl config (default "~/.kube/config")
  -local
    	use this flag when running locally (outside of cluster to use local kube config
```

### Environment variables


`YALE_DEBUG_ENABLED`: set to `true` to enable debug logging
