resource "cloudflare_cloudforce_one_request_priority" "terraform_managed_resource" {
  account_identifier = "023e105f4ecef8ad9ca31a8372d0c353"
  labels = ["DoS", "CVE"]
  priority = 1
  requirement = "DoS attacks carried out by CVEs"
  tlp = "clear"
}
