resource "openpanel_organization" "acme" {
  name     = "Acme"
  timezone = "America/Chicago"
}

output "organization_id" {
  value = openpanel_organization.acme.id
}
