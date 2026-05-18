# Import the SSO config by its parent Organization ID. After import,
# `oidc_client_secret` will be empty in state (the API never returns
# the cleartext). A subsequent `terraform apply` with the secret
# value in the resource config repopulates the write-only attribute.
terraform import openpanel_organization_sso_config.acme <organization-id>
