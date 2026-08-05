# Multi-Tenant Azure Client Secret Support

Yale can hold Azure AD credentials for **N** tenants simultaneously and picks the right one per
`AzureClientSecret` CRD, using the `tenantID` field the CRD already carries.

## Background

Yale used to be single-tenant by construction:

- [`client.go`](../internal/yale/client/client.go)'s `buildAzureGraphClient()` built exactly **one**
  Microsoft Graph client for the whole process lifetime, authenticated via one
  `YALE_APP_REGISTRATION_TENANT_ID` / `YALE_APP_REGISTRATION_CLIENT_ID` env var pair (federated OIDC
  token exchange).
- That one client was reused for every `AzureClientSecret` CRD Yale found cluster-wide.
- The CRD's `spec.azureServicePrincipal.tenantID` field
  ([`azureClientSecret.go`](../internal/yale/crd/api/v1beta1/azureClientSecret.go)) was only ever used
  as `keyops.Key.Scope` — log lines and cache bookkeeping. It never selected a credential.

So Yale could only create and rotate client secrets for applications living in whichever *one* Azure
AD tenant its own credential was scoped to. This mattered because some app registrations live in the
Azure AD **B2C** tenants rather than the non-b2c ARM tenant Yale's original credential was scoped to,
and because Yale doesn't just rotate — it also **creates the first secret** for a new
`AzureClientSecret` CRD, so an unreachable tenant means no working secret at all.

## Configuration

`YALE_APP_REGISTRATION_CREDENTIALS` holds a JSON list of `{tenantId, clientId}` pairs — one per Azure
AD tenant Yale should manage secrets in:

```json
[
  {"tenantId": "<non-b2c tenant id>", "clientId": "<non-b2c client id>"},
  {"tenantId": "<b2c tenant id>", "clientId": "<b2c management app client id>"}
]
```

The list must contain at least one entry; malformed JSON or an empty list is a startup error.

### Legacy fallback

If `YALE_APP_REGISTRATION_CREDENTIALS` is unset, `getYaleAppRegistrationCredentials()` falls back to
the old `YALE_APP_REGISTRATION_TENANT_ID` / `YALE_APP_REGISTRATION_CLIENT_ID` pair and treats it as a
one-entry credential list. That keeps this binary startable against a deployment whose Helm values
haven't been migrated yet (the code and deployment-config rollouts live in different repos and can
land out of order), at the cost of running in effectively single-tenant mode.

This is a temporary shim, not part of the design. Remove `getYaleAppRegistrationCredentials()`'s
legacy branch and the two `yaleLegacy*EnvVar` constants in `client.go` once every cluster is confirmed
to be setting `YALE_APP_REGISTRATION_CREDENTIALS`.

## How it works

**One Graph client per tenant.** `buildAzureGraphClients()` loops over the configured credentials and
calls `buildAzureGraphClient()` once per `{tenantId, clientId}` pair, returning
`map[string]*msgraph.ApplicationsClient` keyed by tenant ID. Each entry does its own federated-OIDC
token exchange — a Google identity token from the GCE/GKE metadata server, audience
`api://AzureADTokenExchange`, exchanged for that tenant's app registration. No secrets are stored for
Yale's own credentials. A failure building any one client fails the whole build, naming the tenant.

**Routing per CRD.** `azKeyOps`
([`azurekeyops.go`](../internal/yale/keyops/azurekeyops/azurekeyops.go)) holds that map and resolves a
client through `clientFor(tenantID)`:

- `Create` routes on its `tenantID` argument (from the CRD).
- `IsDisabled` / `EnsureDisabled` / `DeleteIfDisabled` route on `key.Scope`, which `Create` populates
  with the tenant ID.

A CRD naming a tenant with no configured credential is a loud, specific error listing the tenants Yale
*is* configured for — never a silent skip, and never a rotation against the wrong tenant.

**Local development.** The `local` (`az login`) path builds one client per configured credential the
same way, with `EnableAuthenticatingUsingAzureCLI` instead of OIDC. Note that `TenantID` is set on
`auth.Credentials` for *both* branches: left blank, the Azure CLI authorizer falls back to
`az account show`'s default tenant/subscription, which fails outright for identity-only tenants (e.g.
Azure AD B2C) that have no subscription.

Nothing about Yale's GCP (`GcpSaKey`) code path changed. `yale.go`'s CRD dispatch and the shared
create/disable/delete engine are `keyops.KeyOps`-interface-based and were untouched beyond the one
type change to `newYaleFromClients`' `azure` parameter.

## What each managed tenant's app registration needs

For Yale to authenticate into a tenant, that tenant needs an app registration (one-time, manual — set
up outside this repo) configured with:

- `Application.ReadWrite.All` **application** permission, with tenant-wide admin consent granted.
- A federated credential (Azure's "Other issuer" scenario) per Yale deployment that will authenticate
  against the tenant:
  - **Issuer**: `https://accounts.google.com`
  - **Subject identifier**: the numeric unique ID of that deployment's Yale **GCP service account** —
    Yale federates as a GSA via the metadata server, not a Kubernetes-issued token.
  - **Audience**: `api://AzureADTokenExchange` (must match `azureFederatedCredentialAudience` in
    `client.go` exactly).

One app registration can hold multiple federated credentials, so several Yale deployments sharing a
tenant's app registration each get their own credential entry under the same `clientId`/`tenantId`
pair. The federated-credential count and the `YALE_APP_REGISTRATION_CREDENTIALS` entry count are
independent — don't conflate them.

## Operational note: retiring a CRD's old app registration

Yale's cache keys secrets by `applicationID` ([`entry.go`](../internal/yale/cache/entry.go)), so
repointing a CRD at a different app registration (e.g. moving an app to another tenant) leaves the old
cache entry behind. If the old app registration is deleted in Azure AD first, Yale's auto-retirement
path (`retireCacheEntryIfNeeded` in `yale.go`) never gets to run: `IsDisabled` / `DeleteIfDisabled`
against a nonexistent app registration fail every cycle and the stale entry errors indefinitely.
Delete the old app registration's Yale cache secret in the `yale-cache` namespace **before** deleting
the registration itself, then confirm no further errors reference the old application ID.
