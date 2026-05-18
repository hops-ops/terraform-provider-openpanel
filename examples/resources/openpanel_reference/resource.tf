resource "openpanel_reference" "release_v1_0_0" {
  project_id  = openpanel_project.marketing_site.id
  title       = "Release v1.0.0"
  description = "Initial production launch. Includes the new pricing page."
  date        = "2026-05-18T12:00:00Z"
}
