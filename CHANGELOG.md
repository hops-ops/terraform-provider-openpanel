### What's changed in v0.1.0

* :  (by @patrickleet)

* feat: initial provider — projects, clients, references (by @patrickleet)

  Retargets the hashicorp/terraform-provider-scaffolding-framework
  template at OpenPanel. Wraps the /manage REST API surface
  (apps/api/src/routes/manage.router.ts upstream) shipped in
  Openpanel-dev/openpanel commit 470ddbe8 (2026-01-20).

  Resources:
  - openpanel_project — Project CRUD + import
  - openpanel_client — SDK credential CRUD + import. Sensitive secret
    is returned once at create time and stored in state; subsequent
    Reads do not re-fetch it
  - openpanel_reference — timeline annotation CRUD + import

  Provider configuration (HCL or env):
    host          OPENPANEL_HOST
    client_id     OPENPANEL_CLIENT_ID
    client_secret OPENPANEL_CLIENT_SECRET

  Credentials must belong to a root-typed Client per the upstream
  validateManageRequest auth check.

  Out of scope for v0.1 (need upstream contributions to /manage):
  - openpanel_organization (org create/update/delete is dashboard-only)
  - openpanel_member / openpanel_invitation (member mgmt is dashboard-only)

* chore(examples): replace scaffolding stubs with openpanel resources (by @patrickleet)

  The initial provider commit deleted the example_*.go scaffolding
  under internal/provider/ but missed the parallel cleanup under
  examples/. This commit removes the orphaned scaffolding stubs and
  adds canonical HCL + import.sh files for openpanel_project,
  openpanel_client (showing the create-time secret pattern), and
  openpanel_reference.

  No code change; documentation/examples only.

* feat(auth): OIDC client_credentials and static Bearer auth modes (by @patrickleet)

  Adds two additional authentication modes to the existing root-Client
  header-pair surface:

  1. `oidc { issuer, client_id, client_secret, audience }` — runs the
     OAuth 2.0 client_credentials grant against the configured OIDC
     issuer. Discovers the token endpoint via
     `<issuer>/.well-known/openid-configuration`, caches the JWT until
     ~60s before its expires_in, refreshes transparently on next
     request. Requires the OpenPanel api pod to be configured with
     ADMIN_OIDC_ISSUER (see the openpanel-app fork's feat/admin-jwt-auth
     branch).

  2. `token` — accept a pre-obtained Bearer token (e.g. produced by
     another tool, mounted from a file). The provider passes it through
     as `Authorization: Bearer <token>` without refreshing.

  Existing `client_id` + `client_secret` Client-pair auth remains the
  default and is unchanged for users who haven't configured the other
  modes.

  All three modes are switchable via env vars too — OPENPANEL_TOKEN,
  OPENPANEL_OIDC_{ISSUER,CLIENT_ID,CLIENT_SECRET,AUDIENCE},
  OPENPANEL_CLIENT_{ID,SECRET} — so a Crossplane provider-upjet-openpanel
  ProviderConfig can wire credentials from a Kubernetes Secret without
  templating HCL.

  Implementation:
  - New internal/client/auth.go defines an Authorizer interface +
    three implementations (ClientPairAuth, StaticBearerAuth,
    OIDCClientCredentialsAuth).
  - internal/client/client.go's Client now holds an Authorizer; the
    per-request `do()` calls auth.Authorize(req) to attach credentials.
  - internal/provider/provider.go's Configure picks the right auth
    mode based on what HCL + env supply, with precedence
    token > oidc > client-pair.

* feat: openpanel_organization resource + API shape fixes from verify (by @patrickleet)

  Adds the openpanel_organization resource — the top-level tenant primitive
  inside OpenPanel. Backed by the /manage/organizations REST endpoints
  introduced upstream in PR #371. Verified end-to-end via terraform apply
  against pat-local: an Organization created in TF appears in OpenPanel's
  DB and resolves on subsequent GET.

  Along the way, three API-shape mismatches surfaced from real
  round-trip testing that the unit-style review missed:

  1. /manage controllers wrap responses in `{ data: ... }` — the Go
     HTTP client now unwraps the envelope before decoding into the
     caller-typed struct.

  2. Project.cors and SDKClient.cors are arrays upstream (`z.array(z.string())`),
     not strings. Updated Go types to []string and the corresponding TF
     resource attributes to ListAttribute of types.StringType. Added
     list-helpers (listToStrings / stringsToList) in project_resource.go.

  3. References have asymmetric request/response field naming —
     request body uses `datetime`, response uses `date`. The Reference
     Go type now carries a single Datetime field with custom (Un)Marshal
     so writes serialize as `datetime` and reads accept either name.

  Verified manually on pat-local: openpanel_organization, openpanel_project,
  openpanel_client all create cleanly via terraform apply against the
  existing /manage Client-pair auth surface. Test cluster shows the
  expected rows in Postgres after apply.

* ci: vnext semver tagging + working docs gen + goreleaser hook (by @patrickleet)

  Replace HashiCorp scaffold workflows (`test.yml`, `release.yml`) with
  the same vnext-driven pattern the rest of hops-ops uses:

    - `on-pr.yaml`:           build, lint, docs-up-to-date check on PRs
    - `on-push-main.yaml`:    same quality gates + vnext version-and-tag
                              (uses repo secret DEPLOY_KEY, installed via
                              `vnext generate-deploy-key`)
    - `on-version-tagged.yaml`: GitHub release notes + goreleaser
                              (signed multi-arch binaries for Terraform
                              Registry; needs repo secrets
                              GPG_PRIVATE_KEY + PASSPHRASE)

  Also unblocks `make generate`:
    - `tools/tools.go` was generating docs for `-provider-name scaffolding`;
      update to `openpanel`.
    - `examples/provider/provider.tf` was the scaffold stub
      (`provider "scaffolding"`); replace with the three real auth modes.
    - Drop the stale `docs/example.*` files; regenerated as
      `docs/resources/{client,organization,project,reference}.md`.
    - Add `examples/resources/openpanel_organization/{resource.tf,import.sh}`
      so the new Organization resource has docs.

  Conventional commits will drive tags via vnext from now on. First
  release after this lands becomes v0.1.0 unless commits include
  `feat:`/`fix:` history bumping further.

  Goreleaser secrets and Terraform Registry repo claim are operator
  steps; see follow-up.

* fix(ci): grant packages+issues perms required by vnext reusable workflow (by @patrickleet)

* style: gofmt -s on existing source files (by @patrickleet)


