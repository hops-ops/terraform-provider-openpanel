# Write client — what the front-end JS SDK ships with. Bind this Client's
# id to your build's OPENPANEL_CLIENT_ID; the secret here is the value the
# ingest endpoint validates.
resource "openpanel_client" "web_sdk" {
  project_id = openpanel_project.marketing_site.id
  name       = "Marketing site - Web SDK"
  type       = "write"
  cors       = "https://example.com https://www.example.com"
}

# Root client — required for the Terraform provider itself, /manage API,
# and other admin tooling. Treat this secret like a service-account token.
resource "openpanel_client" "automation" {
  project_id = openpanel_project.marketing_site.id
  name       = "Terraform automation"
  type       = "root"
}

# Secret is returned exactly once on create; emit it to a sensitive
# output so the operator can stash it in their secret manager.
output "web_sdk_secret" {
  value     = openpanel_client.web_sdk.secret
  sensitive = true
}

output "automation_secret" {
  value     = openpanel_client.automation.secret
  sensitive = true
}
