# Multi-Tenant Azure Client Secret Support

## Problem

Yale is single-tenant by construction today:

- [`internal/yale/client/client.go`](../internal/yale/client/client.go)'s `buildAzureGraphClient()` builds exactly **one** Microsoft Graph client for the whole process lifetime, authenticated via one `YALE_APP_REGISTRATION_TENANT_ID` / `YALE_APP_REGISTRATION_CLIENT_ID` env var pair (federated OIDC token exchange).
- That one client is reused for every `AzureClientSecret` CRD Yale finds cluster-wide (`internal/yale/resourcemap/resourcemap.go`'s `listAzureClientSecrets()`).
- The CRD carries its own `TenantID` field (`internal/yale/crd/api/v1beta1/azureClientSecret.go`), but `internal/yale/keyops/azurekeyops/azurekeyops.go`'s `Create()` only ever uses it as `Key.Scope` — log lines and cache bookkeeping. It is never used to select a credential or build a different Graph client.
- `charts/yale/templates/cronjob.yaml` (in `terra-helmfile`) templates `YALE_APP_REGISTRATION_CLIENT_ID`/`_TENANT_ID` straight from a single `.Values.config.azure.clientId`/`.tenantId` scalar pair — there's no list-shaped config today.

In practice, this means Yale can only create/rotate client secrets for applications that live in whichever *one* Azure AD tenant its own credential is scoped to. Checking `terra-helmfile`'s per-cluster values confirms every current Yale deployment (`terra-dev`, `terra-staging`, `terra-qa-bees`, `terra-prod`) points at the "non-b2c" ARM tenant, and that credential is shared across multiple apps. A full sweep of `libchart.azureClientSecret` consumers and cluster-level bundled `azureClientSecrets:` entries (see [Open questions](#open-questions-to-confirm-before-starting)) turned up exactly four consumers, wired up two different ways:
- Sam, Leonardo, and DUOS each define their own `AzureClientSecret` CRD via `yale.azure` in their own chart values (`values/app/{sam,leonardo,duos}/live/*.yaml` + `bee.yaml.gotmpl`), all consuming the shared `libchart.azureClientSecret` template.
- **TDR** is wired up differently: its `datarepo-azure-client-secret` CRD is bundled directly into Yale's *own* chart values (`azureClientSecrets:` in `values/cluster/yale/terra/terra-{dev,staging,prod}.yaml` and `values/cluster/yale/bee-cluster/terra-qa-bees.yaml`), not routed through the per-app libchart template — so it lives in exactly the files Y-D touches.

No other consumers exist in these clusters.

This is blocking DUOS's server-side OAuth (BFF) migration: DUOS's real app registration needs to live in the Azure AD **B2C** tenant (`terradevb2c.onmicrosoft.com` for dev/staging, `terraprodb2c.onmicrosoft.com` for prod) rather than the non-b2c ARM tenant its registration was mistakenly created in. Once that registration moves, Yale needs to be able to reach a second tenant — without breaking Sam's, Leonardo's, or TDR's rotation in the tenant it already manages.

Note this isn't just a rotation gap — Yale also **creates the first secret** for a new `AzureClientSecret` CRD (Terraform never sets one on `azuread_application` resources in this ecosystem). So this work is a hard prerequisite for the relocated DUOS app registration to have a working secret at all, not a "nice to have before the next rotation window."

## Goals

- Yale can hold credentials for **N** Azure AD tenants simultaneously and pick the right one per `AzureClientSecret` CRD, using the `tenantID` field the CRD already carries.
- Zero behavior change for existing single-tenant deployments/apps (Sam, Leonardo, and TDR, all sharing today's non-b2c-tenant credential).
- No change to *how* Yale authenticates (still federated OIDC token exchange, no stored secrets for Yale's own credentials) — only to *how many* credentials it holds.

## Non-goals

- Changing Yale's GCP (`GcpSaKey`) code path — untouched by this work.
- Building a generic "arbitrary number of arbitrary auth methods" framework. This is scoped to "a small, fixed list of tenant/client-id pairs, configured via Helm values."

## Design

`azKeyOps` moves from holding a single `*msgraph.ApplicationsClient` to holding a `map[string]*msgraph.ApplicationsClient` keyed by tenant ID. Each of `Create`/`IsDisabled`/`EnsureDisabled`/`DeleteIfDisabled` looks up the right client using the tenant ID it's already given (`tenantID` param on `Create`, `key.Scope` elsewhere — both already populated from the CRD today, just unused for routing). A CRD naming a tenant with no configured credential is a loud, explicit error — not a silent skip.

`client.go`'s `buildAzureGraphClient()` becomes a loop building one client per configured `{tenantId, clientId}` pair, each doing its own federated-token exchange exactly as it does today for the single pair.

Config surface (`terra-helmfile`): `charts/yale/values.yaml`'s `config.azure.{clientId,tenantId}` becomes a list, e.g.:

```yaml
config:
  azure:
    credentials:
      - clientId: <existing non-b2c client id>
        tenantId: <existing non-b2c tenant id>
      - clientId: <new b2c-tenant management app client id>
        tenantId: <b2c tenant id>
```

`cronjob.yaml` serializes this list into the container (JSON-encoded env var is the lowest-diff option, consistent with how Yale already parses simple env vars; a mounted ConfigMap is the fallback if the value ever needs more structure than that).

## Implementation Plan

### Y-A: Extend Yale's Azure client to hold multiple tenant-scoped clients
**Files:** `internal/yale/client/client.go`
Change `buildAzureGraphClient()` to return a `map[string]*msgraph.ApplicationsClient` keyed by tenant ID, built from a list of `{tenantId, clientId}` credentials (see Y-D). Each entry authenticates independently via the existing federated-OIDC mechanism. Keep the `local` (az-cli) dev path working unchanged.
**Risk:** Low — genuinely additive now. `getYaleAppRegistrationCredentials()` reads `YALE_APP_REGISTRATION_CREDENTIALS` first, and falls back to the legacy `YALE_APP_REGISTRATION_TENANT_ID`/`_CLIENT_ID` pair (as a one-entry credential list) if the new var isn't set — see the cross-repo coordination note under Y-D for why this fallback exists. It's a temporary shim, not a permanent design choice: remove `getYaleAppRegistrationCredentials`'s legacy branch (and the two `yaleLegacy*EnvVar` constants in `client.go`) once all four clusters are confirmed running the new `config.azure.credentials` chart schema.

### Y-B: Route `AzureClientSecret` processing to the correct tenant's client
**Files:** `internal/yale/keyops/azurekeyops/azurekeyops.go` (+ a one-line type change in `internal/yale/yale.go`, where `newYaleFromClients`'s `azure` parameter becomes `map[string]*msgraph.ApplicationsClient` — the `_keyops[azureKeyops] = azurekeyops.New(azure)` call site itself is untouched)
Change `azKeyOps` to hold the tenant→client map and have `Create`/`IsDisabled`/`EnsureDisabled`/`DeleteIfDisabled` select by `tenantID`/`key.Scope`. Fail loudly (clear, specific error) when a CRD's tenant has no matching configured credential.
**Risk:** Low. Confirmed contained to `azurekeyops.go` — `yale.go`'s CRD-type dispatch (the `gcpKeyops`/`azureKeyops` map, the type switch over CRD kinds, the shared Create/disable/delete engine in `yale.go`) is entirely `keyops.KeyOps`-interface-based and untouched by this change. Nothing shared with `GcpSaKey` processing is at risk.

### Y-C: Register the B2C-tenant management app (manual, Azure Portal — one-time per B2C tenant)
Not a code task. Per Microsoft's documented "Automated" pattern for managing B2C tenant resources via Microsoft Graph:
- One management app registered inside `terradevb2c.onmicrosoft.com` — covers dev **and** staging (staging shares the dev B2C tenant, differing only by policy/`p=` value) and BEEs (which reuse dev credentials).
- One management app registered inside `terraprodb2c.onmicrosoft.com` for prod.
- Grant `Application.ReadWrite.All` (application permission) with admin consent in each tenant — **Cloud Application Administrator cannot do this.** Per [Microsoft's admin-consent prerequisites](https://learn.microsoft.com/en-us/entra/identity/enterprise-apps/grant-admin-consent), granting tenant-wide admin consent for an application permission requires Privileged Role Administrator, Global Administrator, or a custom role with the specific permission-grant right — Cloud Application Administrator (and Application Administrator) can create and configure the app registration, but the "Grant admin consent" button itself needs one of those higher roles.
- Add a federated credential trust on each new registration, matching Yale's existing OIDC-federation shape (`api://AzureADTokenExchange` audience, subject bound to the relevant cluster's Yale k8s service account) — same no-stored-secret pattern Yale already uses today.

*Alternative considered:* a single multi-tenant management app registered once, consented across both the B2C and non-b2c tenants, instead of two separate registrations. Rejected — application permissions and admin consent are granted per-tenant regardless of whether the app object itself is single- or multi-tenant, so this buys no reduction in one-time setup steps, while concentrating both tenants' management credentials behind one app increases blast radius if that credential is ever compromised or misconfigured. Two tenant-scoped registrations is the simpler and safer shape.

**Risk:** Low effort, but blocking — nothing downstream works without this, and it needs a specific person's access.

### Y-D: Update Yale's chart schema and per-cluster values
**Files:** `charts/yale/values.yaml`, `charts/yale/templates/cronjob.yaml`, `values/cluster/yale/terra/terra-{dev,staging,prod}.yaml`, `values/cluster/yale/bee-cluster/terra-qa-bees.yaml`
Change `config.azure.{clientId,tenantId}` into a list (`config.azure.credentials: [...]`), update `cronjob.yaml` to serialize it into the container, and migrate all four existing per-cluster values files to the new schema in the same change (this repo — `terra-helmfile` — is the single source of truth for every Yale *deployment config*, so there's no second config repo to keep in sync). Add the new B2C-tenant credential to dev/staging/qa-bees and prod.

Note this does **not** mean the overall rollout is single-repo: Y-A/Y-B live in `yale`, Y-D/Y-E live here, and the real DUOS B2C app registration comes from `terraform-ap-deployments`. Worse, Yale's own CI dispatches an automated version bump into this repo on merge (`.github/workflows/update_service.yaml`, `repository_dispatch`) — fully decoupled from any `terra-helmfile` PR. If the `yale` PR (Y-A/Y-B) merges before this PR (Y-D) merges, that automated bump would roll out a Yale image into clusters still running the old two-env-var chart. This no longer crash-loops Yale, because `yale`'s `client.go` accepts the old `YALE_APP_REGISTRATION_TENANT_ID`/`_CLIENT_ID` pair as a one-entry fallback when `YALE_APP_REGISTRATION_CREDENTIALS` is absent (see Y-A) — but until this PR (Y-D) lands, Yale is still running in single-tenant mode regardless of merge order, so DUOS's B2C secret can't be created until both PRs are in.
**Risk:** Low-medium — must not regress Sam or Leonardo (whose CRDs authenticate via this same credential from their own app values files), or **TDR**, whose `datarepo-azure-client-secret` entry is bundled directly inside these same per-cluster values files being edited here.

### Y-E: Point DUOS's own CRD at the new tenant and app registration
**Files:** `values/app/duos/live/{dev,staging,prod}.yaml`, `values/app/duos/bee.yaml.gotmpl`
Add a DUOS-specific `global.azure.tenantID` override (the B2C tenant — DUOS doesn't override this today, it inherits the shared non-b2c default from `values/app/global/live/*.yaml`) and update `azure.applicationID` to the new app registration's client ID once it exists (from the corresponding `terraform-ap-deployments` work). Isolated change — the global default other apps inherit is untouched.
**Risk:** Low.

### Y-F: Validate in dev before staging/prod
Y-A/Y-B's legacy-env-var fallback (see Y-A) means the `yale` and `terra-helmfile` PRs no longer have to land in the same window — Y-A/Y-B alone is safely deployable ahead of Y-D, it just runs in single-tenant mode (identical to today) until Y-D lands. Full multi-tenant capability still needs both. Sequence:
1. Confirm the B2C tenant/application IDs and per-cluster GSAs are all verified (see Open questions).
2. Complete Y-C (management app + all required federated credentials) in the dev B2C tenant.
3. Deploy Y-A/Y-B + Y-D to `terra-dev`, with DUOS's CRD still pointed at its old (non-b2c) app registration — i.e. land the multi-tenant capability before using it for anything.
4. Confirm Sam's, Leonardo's, and TDR's (`datarepo-azure-client-secret`, bundled directly in `terra-dev`'s own Yale values) existing rotation is unaffected — this is the real regression gate, since Y-A/Y-B change a shared code path for every app Yale manages in that cluster.
5. Only once step 4 is clean, apply Y-E to point DUOS's CRD at the new B2C tenant/app registration. Confirm Yale successfully **creates** (not just rotates) the first secret for the new DUOS dev app registration and writes it to `duos-azure-client-secret`.
6. Clean up the old DUOS app registration's residue: Yale's cache keys secrets by `applicationID` (`internal/yale/cache/entry.go`), so switching DUOS's CRD to the new app registration leaves the *old* cache entry behind. If the old app registration is later deleted in Azure AD, Yale's normal auto-retirement path (`retireCacheEntryIfNeeded`, `yale.go`) never gets a chance to run — `IsDisabled`/`DeleteIfDisabled` calls against the now-nonexistent app registration fail every cycle, and the stale cache entry (with its old `CurrentKey`) is stuck erroring indefinitely instead of being cleaned up automatically. Before deleting the old app registration, manually delete its Yale cache secret in the `yale-cache` namespace and confirm no more errors reference the old application ID.
7. Only after dev is clean end-to-end, roll Y-D/Y-E to staging, then prod, repeating steps 2 and 6 per environment.
**Risk:** This is the real regression gate, not a formality — Y-A/Y-B change a shared code path for every app Yale manages in that cluster, and step 6 has no existing tooling (no CLI command or documented manual procedure exists yet for cleaning up a stale Yale cache entry).

## Open questions to confirm before starting

- ~~Whether any `AzureClientSecret` exists in these clusters beyond Sam/Leonardo/DUOS~~ — **Resolved.** A full sweep of `libchart.azureClientSecret` consumers (`grep` for `applicationID` under `values/app/*/live/*.yaml`, `live.yaml.gotmpl`, `bee.yaml.gotmpl`) and cluster-level bundled `azureClientSecrets:` lists (`values/cluster/yale/**`) found exactly one more consumer beyond Sam/Leonardo/DUOS: **TDR**, via its `datarepo-azure-client-secret` entry bundled directly in Yale's own per-cluster values (all four clusters), not routed through the shared libchart template the other three use. No other consumers exist. The Problem section and Y-D/Y-F above have been updated to account for TDR explicitly.
- The staging-shares-dev-B2C-tenant assumption traces to a footnote in the DUOS BFF migration plan itself flagged as unverified ("verify the staging and prod B2C policy names and tenant URLs... before enabling those environments") — confirm directly in the Azure portal before Y-C.
- Exact env-var vs. mounted-config-file choice for Y-A/Y-D depends on constraints in the Helm chart not yet fully audited (secret size limits, existing patterns elsewhere in the chart for list-shaped config).

## Appendix: Y-C step-by-step (Azure Portal)

Y-C's summary above says the federated credential's subject is "bound to the relevant cluster's Yale k8s service account." That's not what [`client.go`](../internal/yale/client/client.go)'s `buildAzureGraphClient()` actually does: Yale gets its OIDC token via `google.FindDefaultCredentials` + `idtoken.NewTokenSource` (the GCE/GKE metadata server), i.e. it authenticates as a **GCP service account**, with `https://accounts.google.com` as the OIDC issuer — not a Kubernetes-issued token. The federated credential configured below trusts a Google service account identity, not a k8s SA subject.

**There is no shared GSA across clusters.** Checked directly against `charts/yale/templates/serviceAccount.yaml`'s Workload Identity annotation (`yale-{clusterNick}@{googleProject}.iam.gserviceaccount.com`) and each cluster's `googleProject`/`clusterNick` values: dev, staging, qa-bees, and prod each resolve to a **fully distinct GSA**:
- `yale-dev@broad-dsde-dev.iam.gserviceaccount.com`
- `yale-staging@broad-dsde-staging.iam.gserviceaccount.com`
- `yale-qa-bees@broad-dsde-qa.iam.gserviceaccount.com`
- `yale-prod@broad-dsde-prod.iam.gserviceaccount.com`

Since dev, staging, and qa-bees all share the same *app registration* in `terradevb2c.onmicrosoft.com` (one `clientId`/`tenantId` pair per Y-D's config), but each runs as its own distinct GSA, that one app registration needs **three separate federated credential trust entries** — one per GSA subject, all pointing at the same app object. An Azure AD app registration supports multiple federated credentials simultaneously, so this is additive within Step 4, not three separate app registrations. The prod app registration needs just one (its own GSA).

### Prerequisites

- **Access**: **Privileged Role Administrator** or **Global Administrator** (or a custom role with the permission-grant right) *in the specific B2C tenant* — separate from any role in Yale's usual non-b2c ARM tenant. **Cloud Application Administrator is not sufficient**: it can create and configure the app registration, but cannot grant tenant-wide admin consent for an application permission (see [Microsoft's admin-consent prerequisites](https://learn.microsoft.com/en-us/entra/identity/enterprise-apps/grant-admin-consent)).
- **Info to have on hand**:
  - The GSA email for each cluster that will authenticate against this tenant — all three (`yale-dev`, `yale-staging`, `yale-qa-bees`) for the dev-tenant pass, just `yale-prod` for the prod-tenant pass.
  - Each GSA's **numeric unique ID** (Client ID) — via `gcloud iam service-accounts describe <gsa-email> --format='value(uniqueId)'`, or GCP Console → IAM & Admin → Service Accounts → click the account → "Unique ID".

Do the whole procedure **twice** — once in `terradevb2c.onmicrosoft.com` (registering 3 federated credentials, one per GSA), once in `terraprodb2c.onmicrosoft.com` (registering 1).

### Step 1: Switch to the correct Azure AD B2C tenant

1. Go to https://portal.azure.com and sign in.
2. Click your account icon (top right) → **Switch directory**.
3. Pick `terradevb2c.onmicrosoft.com` (or `terraprodb2c.onmicrosoft.com` for the prod pass). If it's not listed, you don't have access yet — get that first.
4. Confirm: search the top bar for **"Azure AD B2C"** or **"Microsoft Entra ID"** and open it — the **Tenant ID** on the Overview page should match the B2C tenant you intend.

### Step 2: Register the app

1. Search bar → **"App registrations"** → **+ New registration**.
2. Fill in:
   - **Name**: something identifying, e.g. `yale-b2c-tenant-management` (same name works in both tenants, since they're separate app objects).
   - **Supported account types**: leave the default, **"Accounts in this organizational directory only (single tenant)"** — this app only needs to act within its own B2C tenant.
   - **Redirect URI**: leave blank. This is a daemon/service app (federated-credential token exchange), not an interactive sign-in app.
3. Click **Register**.
4. On the app's **Overview** page, copy and save two values for Y-D:
   - **Application (client) ID**
   - **Directory (tenant) ID**

### Step 3: Grant `Application.ReadWrite.All` with admin consent

1. App's left nav → **API permissions** → **+ Add a permission**.
2. Choose **Microsoft Graph** → **Application permissions** (not Delegated — there's no signed-in user here).
3. Search `Application.ReadWrite.All`, check it, **Add permissions**.
4. Click **Grant admin consent for `<tenant name>`** → **Yes**.
5. Verify the status column shows a green checkmark ("Granted for `<tenant>`"). If the button is greyed out or the "Grant admin consent" action isn't available, your account doesn't hold Privileged Role Administrator or Global Administrator (or an equivalent custom role) in this tenant — Cloud Application Administrator alone won't show this option.

### Step 4: Add a federated credential per GSA (no stored secret)

One app registration can — and here, must — hold multiple federated credentials. In the dev-tenant pass, repeat this step three times (once per GSA: `yale-dev`, `yale-staging`, `yale-qa-bees`); in the prod-tenant pass, do it once (`yale-prod`).

For each GSA:
1. App's left nav → **Certificates & secrets** → **Federated credentials** tab → **+ Add credential**.
2. **Federated credential scenario**: **Other issuer** (no built-in "GCP" preset — Google is a generic OIDC issuer here).
3. Fill in:
   - **Issuer**: `https://accounts.google.com`
   - **Subject identifier**: that GSA's numeric **Unique ID**.
   - **Name**: descriptive and distinguishable from the others on this same app, e.g. `yale-dev-gsa`, `yale-staging-gsa`, `yale-qa-bees-gsa`, or `yale-prod-gsa`.
   - **Audience**: `api://AzureADTokenExchange` (must match exactly — the constant in [`client.go`](../internal/yale/client/client.go)).
4. Click **Add**.

### Step 5: Repeat for the other tenant

Switch directory to the other B2C tenant (Step 1) and repeat Steps 2–4 (with that tenant's GSA(s) — 3 for dev, 1 for prod).

### Output for Y-D

For each tenant, you should end up with:

```yaml
- clientId: <Application (client) ID from Step 2>
  tenantId: <Directory (tenant) ID from Step 2>
```

Two such entries (dev/staging/qa-bees pointing at the dev B2C tenant's registration, prod pointing at the prod one) go into `charts/yale/values.yaml` per Y-D — the same `clientId`/`tenantId` pair is shared by dev, staging, and qa-bees, since they authenticate as the same app registration (just via three separate federated credentials, one per GSA, from Step 4). The federated credential count (4) and the config entry count (2) are deliberately different things — don't conflate them when filling in Y-D.
