### What's changed in v0.2.0

* docs(readme): real OpenPanel provider readme with examples, auth modes, related repos (by @patrickleet)

* feat: openpanel_organization_sso_config resource (by @patrickleet)

  Adds a Terraform resource for per-organization OIDC SSO config. Hits
  the new `/manage/organizations/:id/sso` REST endpoints from the
  openpanel-app fork's feat/per-org-sso branch.

  Behavior
  --------
  - 1:1 with `openpanel_organization`. `organization_id` is
    `RequiresReplace` so the relation is enforced.
  - `oidc_client_secret` is `Sensitive` and write-only: the cleartext
    crosses TLS once on apply, the server encrypts at rest, and reads
    return `has_oidc_client_secret: bool` instead of the value. The
    Read handler explicitly preserves the in-state secret to avoid
    bumping plan-drift on every refresh.
  - `enforced_for_domains` is a `list(string)`; bare DNS labels
    validated server-side.
  - Import accepts the organization_id; post-import a fresh apply
    re-injects the cleartext secret into state.

  Client — internal/client/client.go
  ----------------------------------
  - `OrgSsoConfig` struct mirrors the wire shape including the
    computed `HasOidcClientSecret` flag.
  - `GetOrgSsoConfig` / `UpsertOrgSsoConfig` / `DeleteOrgSsoConfig`
    against `/manage/organizations/:id/sso`. PUT serves both create
    and update; server enforces the four required fields on first
    write.

  Schema quirk
  ------------
  The TF Plugin Framework reserves `provider` as an attribute name
  (it's the block-aliased provider-instance attribute). The resource
  exposes the field as `provider_type` instead; the API contract on
  the wire stays `provider`.

  Example + import.sh added under examples/resources/. Docs
  regenerated via `make generate`.

  Build clean: `go build ./...` and `make generate` both pass.

  Next: bump TERRAFORM_PROVIDER_VERSION in provider-openpanel after
  this lands as a tagged release; the upjet codegen picks up the new
  resource automatically.


See full diff: [v0.1.0...v0.2.0](https://github.com/hops-ops/terraform-provider-openpanel/compare/v0.1.0...v0.2.0)
