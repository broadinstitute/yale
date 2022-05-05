# yale

[![Go Report Card](https://goreportcard.com/badge/github.com/broadinstitute/yale)](https://goreportcard.com/report/github.com/broadinstitute/yale)

## Usage

Yale is a Go service that manages Google Cloud Platform (GCP) service account (SA) keys used by Kubernetes resources. As stated in  GCP documents, <em>Service accounts are unique identities used to facilitate programmatic access to GCP APIs</em>. For compliance, keys must be rotated no more than 90 days.


Yale has five purposes:
1. Create new secrets for new GSK resources and store referenced SA keys in a Secrets.
2. Detect keys that need to be rotated.
3. Update GSK generated Secrets with new keys.
4. Check for old keys and disable them.
5. Delete disabled keys.

Yale monitors GcpSaKey resouces to manage the referenced GCP SA. GcpSaKey is custom resource definition, or [CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), that tells Yale how to create or modify secrets holding GCP SA keys. These secrets are referenced in Kubernetes resources, ie. Deployments, Cronjobs, etc, and when modified, trigger rolling updates to update those resources.

## How to use Yale

A GcpSaKey needs to be created via helm for every service account. For example,

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
or if using default values:
```
apiVersion: yale.broadinstitute.org/v1beta1
kind: GcpSaKey
metadata:
  name: example-gcpsakey
spec:
  secret:
      name: example-sa-secret
    googleServiceAccount:
      name: example@broad-dsde-dev.iam.gserviceaccount.com
      project: broad-dsde-dev
```

Where:

| Field | Type | Required| Default | Description |
|-----|------|------|---------|-------------|
| metadata.name| string| yes | | Name of Resource. **Name must end in gcpsakey**|
| spec.secret.name | string | yes|  | Name of Secret that houses SA. **Name must end in "sa-secret"** |
|spec.secret.pemKeyName | string |  no | service-account.pem | Name of Secret data field that stores pem private key|
| spec.secret.jsonKeyName | string | no | service-account.json | Name of Secret data field that stores private key |
| spec.keyRotation.rotateAfter | int | no | 69 | Amount of days before key is rotated |
| spec.keyRotation.deleteAfter | int | no | 14 | Amount of days key is disabled before deleting |
| spec.keyRotation.disableAfter | int | no | 10 | Amount of days since key was last authenticated against before disabling |
| spec.googleServiceAccount.name | string | yes |  | Email of the GCP SA |
| spec.googleServiceAccount.project | string | yes |  | Google project ID SA is associated with|

That's all! Yale takes care of the rest!

## Installation

Yale is intended to be deployed as a kubernetes cronjob. Since keys rotate at the project level, not service, the cronJob performs all functions and checks against SA keys defined in GcpSaKey resource. Therefore, the cronJob does not need to be added to each service. The cronjob will run Yale every 2 minutes.

When deployed as a cronjob [via helm](https://github.com/broadinstitute/terra-helmfile/blob/master/charts/yale/templates/cronJob.yam), Yale uses in cluster authentication provided by kubernetes/client-go. There are no additional steps required to configure this.

Yale also requires a GCP service account with roles/iam.serviceAccountKeyAdmin role. The cronjob expects service account credentials to be mounted to the pod or workload identity can be used instead.

### Running Locally

While the intended use for yale is to run as a kubernetes cronjob it is also possible to run the tool locally against a remote cluster.
A public docker image is available at `us-central1-docker.pkg.dev/dsp-artifact-registry/yale/yale:v0.0.12`

When running the docker image locally the `-local` runtime flag must be used. This tells yale to connect to a remote cluster using your local `.kube/config` otherwise in cluster authentication will be used. Your local `.kubconfig` and a GCP credential must be mounted to the container when running locally.

Unit tests can be run with `go test`:

```
    # Run tests w/ coverage stats
    go test -coverprofile=coverage.out

    # View line-by-line coverage report in browser
    go tool cover -html=coverage.out
```

### Runtime flags

```
Usage of yale:
  -kubeconfig string
    	(optional) absolute path to kubectl config (default "~/.kube/config")
  -local
    	use this flag when running locally (outside of cluster to use local kube config
```
