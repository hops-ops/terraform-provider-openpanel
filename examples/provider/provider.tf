# Client-pair auth (default; works against any OpenPanel install once you
# have a root-typed Client. The chart-managed bootstrap Job mints one at
# install time and writes the credential to the openpanel-bootstrap-root
# Secret.)
provider "openpanel" {
  host          = "https://analytics.example.com"
  client_id     = var.openpanel_client_id
  client_secret = var.openpanel_client_secret
}

# Or: OIDC client_credentials grant against an OIDC issuer
# (requires the openpanel-app fork's admin-JWT auth middleware).
#
# provider "openpanel" {
#   host = "https://analytics.example.com"
#
#   oidc {
#     issuer        = "https://auth.example.com"
#     client_id     = var.oidc_client_id
#     client_secret = var.oidc_client_secret
#     audience      = "openpanel-admin"
#   }
# }

# Or: pre-obtained static JWT (e.g. from another tool).
#
# provider "openpanel" {
#   host  = "https://analytics.example.com"
#   token = var.openpanel_token
# }
