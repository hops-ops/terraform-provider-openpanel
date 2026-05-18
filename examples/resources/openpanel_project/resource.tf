resource "openpanel_project" "marketing_site" {
  name   = "Marketing site"
  domain = "example.com"
  # Space-separated origins allowed to ingest events via the SDK.
  cors = "https://example.com https://www.example.com"
}

output "project_id" {
  value = openpanel_project.marketing_site.id
}
