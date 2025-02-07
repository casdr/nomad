# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: BUSL-1.1

terraform {
  required_providers {
    enos = {
      source = "registry.terraform.io/hashicorp-forge/enos"
    }
  }
}

locals {
  binary_destination = var.platform == "windows" ? "C:/opt/" : "/usr/local/bin/"
  ssh_user           = var.platform == "windows" ? "Administrator" : "ubuntu"
}

resource "enos_bundle_install" "nomad" {
  destination = local.binary_destination

  artifactory = var.artifactory_release

  transport = {
    ssh = {
      host             = var.server_address
      private_key_path = var.ssh_key_path
      user             = local.ssh_user
    }
  }
}

resource "enos_remote_exec" "restart_linux_services" {
  count      = var.platform == "linux" ? 1 : 0
  depends_on = [enos_bundle_install.nomad]


  transport = {
    ssh = {
      host             = var.server_address
      private_key_path = var.ssh_key_path
      user             = local.ssh_user
    }
  }

  inline = [
    "sudo systemctl restart nomad",
  ]
}

resource "enos_remote_exec" "restart_windows_services" {
  count      = var.platform == "windows" ? 1 : 0
  depends_on = [enos_bundle_install.nomad]

  transport = {
    ssh = {
      host             = var.server_address
      private_key_path = var.ssh_key_path
      user             = local.ssh_user
    }
  }

  inline = [
    "powershell Restart-Service Nomad"
  ]
}
