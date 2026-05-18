# Note: importing a Client populates state but cannot recover the
# `secret` attribute — OpenPanel does not return it on subsequent reads.
# Operators who need the secret post-import must either store it
# out-of-band at create time or recreate the Client.
terraform import openpanel_client.web_sdk <client-id>
