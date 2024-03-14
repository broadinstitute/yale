# Yale Design

This repo is responsible for housing the following information:

- A Golang based Kubernetes Daemon intended to be run as a scheduled cronjob
- A linter for instances of the Yale CRD yaml files
- tests, k8s `CustomResourceDefinitions`
- various scripts/ to support various repo-operations

Yale's core logic is that it will list out any relevant CRDs it is aware of in the cluster
and then determine what actions it needs to take upon the secret described within.

These actions can be:
1. Create a new secret
2. Disable an existing secret if the backing platform supports a disabled state
3. Delete the secret
4. Do nothing

The process of rotation consists of creating a new secret and propagating it out to any desired locations
and then disabling the old one. After an amount of time specified in the crd the old secret will be deleted entirely.


## Caching

Yale implements a cache of the secrets it manages using kubernetes secrets in a separate kubernetes namespace.
This is to address the problem of Terra's Bee environments where we have dozens of instances of the same application
each with Yale CRDs that all reference the same service account.

In other words it uses kubernetes native `Secret` resource type as the persistence layer for the cache

## Repo Layout

cmd/

Contains the `main` functions for each binary contained within this project
This is a common pattern in the Go community, the main functions don't really do much
outside of invoking the core application logic in `/internal`

crd/

Contains the CustomResourceDefinitions for the CRDs that yale monitors in order to perform it's core business logic

examples/

Contains example instances of the various CRDs that yale supports

scripts/

Utility Scripts to facilitate developer interactions with repository

internal/

Contains all the code for the applications in this repo.

internal/linter/

Contains the code for the linter tool that is used in validate Yale CRDs in helm CI/CD pipelines

internal/yale/
All the application code files for yale live under this folder

internal/yale/yale.go/

This file contains Yale's core runtime logic that it iterates through, all the other folders in this directory contain code that supports this main routine
If you are a new contributor, start here.

internal/yale/authmetrics/

Library code for determining if a Yale managed secret is still being actively used to authenticate

internal/yale/cache/

Libary code implementing the logic for Yale to use k8s builtin secret types as a caching mechanism

internal/yale/client/

Logic for initializing client objects for any third party dependencies such as GCP, Azure, slack, etc..

internal/yale/crd/

Logic for parsing Yale's k8s CRD specs and translating them into domain types

internal/yale/keyops/

Logic for implementing the CRUD operations that Yale needs to perform on the secrets it manages.
The code in this package abstracts directly working with the client objects in /clients by providing
a `Keyops` interface that a client must implement.

internal/yale/keysync/

Logic for taking a yale managed secret and propagating it out to other destinations including the cache
as well as Vault.

internal/yale/logs/

A really hacky levelled logging implementation. A future nice to have would be to replace this with a real
logging library.

internal/yale/resourcemap/

Defines a method for associating a particular instance of a yale CRD with all the raw k8s secrets, cache entries, and vault secrets that were generated from it.

internal/yale/slack/

Utilities for sending messages to slack
