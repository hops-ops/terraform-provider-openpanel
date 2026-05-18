# Terraform Provider: OpenPanel

A Terraform provider for [OpenPanel](https://openpanel.dev) — the
self-hosted, open-source product analytics platform. Manages OpenPanel
**Organizations**, **Projects**, **Clients**, and **References** via
the `/manage` REST API.

Use this provider to:

- Declaratively provision per-tenant Organizations and Projects from
  CI / pull-requests rather than the OpenPanel dashboard.
- Wire OpenPanel into a Crossplane-managed platform: this provider is
  the upstream for [`provider-upjet-openpanel`][upjet], the Crossplane
  provider generated from it.
- Mint ingest-only Client credentials per environment from the same
  IaC tree that defines your apps.

[upjet]: https://github.com/hops-ops/provider-upjet-openpanel

## Status

| | |
|---|---|
| Tracks OpenPanel | [`hops-ops/openpanel-app`](https://github.com/hops-ops/openpanel-app) fork (admin-JWT auth + `/manage/organizations` endpoints; both pending upstream as [openpanel-dev/openpanel#371](https://github.com/openpanel-dev/openpanel/pull/371)) |
| Auth modes | Client-pair (any OpenPanel install), OIDC `client_credentials`, static Bearer |
| Latest release | See [Releases](https://github.com/hops-ops/terraform-provider-openpanel/releases) |

> The provider speaks the standard `/manage` REST API for Projects,
> Clients, and References (available in upstream OpenPanel since
> 2026-01-20). The Organizations resource and the OIDC + Bearer auth
> modes require the hops-ops fork's admin-JWT middleware until that
> patch lands upstream.

## Installing

### From the Terraform Registry

```hcl
terraform {
  required_providers {
    openpanel = {
      source  = "hops-ops/openpanel"
      version = "~> 0.1"
    }
  }
}
```

### Locally (for development)

Set up [`dev_overrides`][dev-overrides] in `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "hops-ops/openpanel" = "/path/to/your/$GOPATH/bin"
  }
  direct {}
}
```

Then `go install` from a clone of this repo.

[dev-overrides]: https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers

## Authentication

The provider supports three auth modes. Configure exactly one.

### Client-pair (default — works against any OpenPanel install)

Mint a root-typed Client through the OpenPanel dashboard (or via the
[openpanel-chart][chart] Helm bootstrap Job, which writes the
credential to a chart-managed `openpanel-bootstrap-root` Secret at
install time).

```hcl
provider "openpanel" {
  host          = "https://analytics.example.com"
  client_id     = var.openpanel_client_id
  client_secret = var.openpanel_client_secret
}
```

Environment fallback: `OPENPANEL_HOST`, `OPENPANEL_CLIENT_ID`,
`OPENPANEL_CLIENT_SECRET`.

[chart]: https://github.com/hops-ops/openpanel-chart

### OIDC `client_credentials` (fork-only)

Lets the provider authenticate via your platform IdP (Zitadel, Okta,
etc.) instead of a long-lived shared secret. Requires the hops-ops
fork's `ADMIN_OIDC_ISSUER` configuration.

```hcl
provider "openpanel" {
  host = "https://analytics.example.com"

  oidc {
    issuer        = "https://auth.example.com"
    client_id     = var.oidc_client_id
    client_secret = var.oidc_client_secret
    audience      = "openpanel-admin"
  }
}
```

The provider runs the `client_credentials` grant against
`<issuer>/.well-known/openid-configuration`, caches the JWT, and
refreshes ~60s before expiry.

### Static Bearer

When some external tool already issues a JWT:

```hcl
provider "openpanel" {
  host  = "https://analytics.example.com"
  token = var.openpanel_token
}
```

## Resources

### `openpanel_organization`

Top-level tenant primitive; scopes Projects, Clients, References,
Members.

```hcl
resource "openpanel_organization" "acme" {
  name     = "Acme"
  timezone = "America/Chicago"
}
```

### `openpanel_project`

```hcl
resource "openpanel_project" "marketing_site" {
  name   = "Marketing site"
  domain = "example.com"
  cors   = "https://example.com https://www.example.com"
}
```

### `openpanel_client`

Per-environment ingest credentials. Use Client `type = "write"` for
SDKs; `type = "root"` for management API access.

```hcl
resource "openpanel_client" "marketing_site_web" {
  name       = "Marketing site web"
  type       = "write"
  project_id = openpanel_project.marketing_site.id
  cors       = "https://example.com"
}

output "marketing_site_client_id" {
  value = openpanel_client.marketing_site_web.id
}

output "marketing_site_client_secret" {
  value     = openpanel_client.marketing_site_web.secret
  sensitive = true
}
```

### `openpanel_reference`

Free-form metadata for slicing analytics (release tags, deploys,
feature flags, etc.).

```hcl
resource "openpanel_reference" "release_v1_2_0" {
  project_id = openpanel_project.marketing_site.id
  name       = "v1.2.0"
  date       = "2026-05-18T00:00:00Z"
  description = "Marketing-site relaunch"
}
```

## Full reference

Per-resource attribute documentation lives under [`docs/`](./docs/);
the Terraform Registry surfaces the same content rendered with
examples.

## Development

### Requirements

- Go ≥ 1.24
- Terraform ≥ 1.13

### Build & generate

```shell
go install
make generate    # regenerates docs/ from the provider schema
```

### Tests

```shell
make test     # unit
make testacc  # acceptance (creates real resources against a real OpenPanel; needs OPENPANEL_HOST etc.)
```

### Release

Releases are driven by [vnext][vnext] from conventional commits on
`main`. Pushing a `feat:` / `fix:` commit to main:

1. CI runs build + lint + docs-up-to-date checks.
2. `vnext` calculates the next semver and pushes a tag.
3. The `on-version-tagged` workflow runs goreleaser, signs the
   binaries with the repo's GPG key, and publishes them as a GitHub
   Release. The Terraform Registry picks the release up automatically.

[vnext]: https://github.com/unbounded-tech/vnext

## Related repos

| Repo | Purpose |
|---|---|
| [`hops-ops/openpanel-app`](https://github.com/hops-ops/openpanel-app) | OpenPanel fork with admin-JWT auth + `/manage/organizations` endpoints |
| [`hops-ops/openpanel-chart`](https://github.com/hops-ops/openpanel-chart) | Helm chart with per-concern Secrets + root-Client bootstrap Job |
| [`hops-ops/analytics-stack`](https://github.com/hops-ops/analytics-stack) | Crossplane composition that runs the chart + publishes the bootstrap credential to AWS Secrets Manager |
| [`hops-ops/provider-upjet-openpanel`](https://github.com/hops-ops/provider-upjet-openpanel) | Crossplane provider generated from this Terraform provider via upjet |

## License

MPL-2.0
