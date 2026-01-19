# Terraform and Provider Version Constraints
# AnyHost Oracle Cloud Always Free Deployment

terraform {
  required_version = ">= 1.0"

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = ">= 5.0"
    }
  }
}

# Configure the OCI provider
# Authentication is handled via OCI CLI config file (~/.oci/config)
# Or via environment variables:
#   - OCI_TENANCY_OCID
#   - OCI_USER_OCID
#   - OCI_FINGERPRINT
#   - OCI_PRIVATE_KEY_PATH
#   - OCI_REGION
provider "oci" {
  # Uses default profile from ~/.oci/config
  # Or specify: config_file_profile = "DEFAULT"
}
