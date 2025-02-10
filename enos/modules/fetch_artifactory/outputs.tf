# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: BUSL-1.1

output "nomad_local_binary" {
  description = "Path where the binary will be placed"
  value       = var.os == "windows" ? "${var.download_binary_path}/nomad.exe" : "${var.download_binary_path}/nomad"
}

output "artifact_url" {
  description = "URL to fetch the artifact"
  value       = data.enos_artifactory_item.nomad.results[0].url
}

output "artifact_sha" {
  description = "sha256 to fetch the artifact"
  value       = data.enos_artifactory_item.nomad.results[0].sha256
}
