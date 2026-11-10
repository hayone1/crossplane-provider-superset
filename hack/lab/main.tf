# A temporary Hetzner Cloud server for building and testing the provider: it runs Superset
# in Docker, a kind cluster with Crossplane, and the Go builds that need more memory than a
# laptop has. See hack/lab/README.md.

terraform {
  required_version = ">= 1.5"
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.52"
    }
  }
}

variable "hcloud_token" {
  description = "Hetzner Cloud API token (TF_VAR_hcloud_token)."
  type        = string
  sensitive   = true
}

variable "name" {
  description = "Name of the server, its SSH key and its firewall."
  type        = string
  default     = "superset-lab"
}

variable "server_type" {
  description = "Hetzner server type. 16 vCPUs and 32 GB build the generated code in a few minutes."
  type        = string
  default     = "cx53"
}

variable "location" {
  description = "Hetzner location."
  type        = string
  default     = "nbg1"
}

variable "ssh_public_key_file" {
  description = "Public half of the key that may log in as root."
  type        = string
}

variable "allowed_ssh_cidrs" {
  description = "Networks that may reach port 22. Nothing else is open."
  type        = list(string)
}

provider "hcloud" {
  token = var.hcloud_token
}

resource "hcloud_ssh_key" "lab" {
  name       = var.name
  public_key = file(var.ssh_public_key_file)
}

resource "hcloud_firewall" "lab" {
  name = var.name
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = var.allowed_ssh_cidrs
  }
}

resource "hcloud_server" "lab" {
  name         = var.name
  server_type  = var.server_type
  location     = var.location
  image        = "ubuntu-24.04"
  ssh_keys     = [hcloud_ssh_key.lab.id]
  firewall_ids = [hcloud_firewall.lab.id]
  user_data    = file("${path.module}/cloud-init.yaml")
  labels       = { purpose = "crossplane-provider-superset-lab" }
}

output "ipv4" {
  value = hcloud_server.lab.ipv4_address
}
