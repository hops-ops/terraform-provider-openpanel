resource "openpanel_organization" "acme" {
  name = "Acme"
}

resource "openpanel_organization_sso_config" "acme" {
  organization_id = openpanel_organization.acme.id

  display_name = "Acme SSO"

  # OIDC client issued by your IdP (Zitadel, Keycloak, Okta, …).
  # Cleartext client_secret crosses TLS once on apply; the server
  # encrypts it at rest and never returns it on subsequent reads.
  oidc_client_id     = var.acme_oidc_client_id
  oidc_client_secret = var.acme_oidc_client_secret

  oidc_authorization_endpoint = "https://auth.acme.com/oauth/v2/authorize"
  oidc_token_endpoint         = "https://auth.acme.com/oauth/v2/token"
  oidc_jwks_uri               = "https://auth.acme.com/oauth/v2/keys"

  # `/login` routes users whose email is at one of these domains to
  # Acme's IdP. When `is_required` is true, members of the Acme
  # organization cannot fall back to email/password — useful once
  # the flow has been verified end-to-end.
  enforced_for_domains = ["acme.com", "acme.co.uk"]
  is_required          = false
}
