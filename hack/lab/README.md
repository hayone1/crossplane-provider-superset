# Test lab on Hetzner Cloud

This folder creates a temporary server on Hetzner Cloud for testing the provider. The server runs
Superset in Docker, kind clusters, and the Go builds, which need more memory than a small laptop
has. Only your address may reach it, through SSH. It costs about EUR 0.25 an hour, so destroy it
when you are done.

## Create the lab

1. Put your Hetzner Cloud API token into `.env` in the repository root, as
   `TF_VAR_hcloud_token=...`.
2. Create an SSH key and a variables file in the git-ignored `_debug/lab` folder. Replace the
   address with your own public address.

   ```shell
   mkdir -p _debug/lab
   ssh-keygen -q -t ed25519 -N '' -f _debug/lab/id_ed25519
   cat > _debug/lab/lab.tfvars <<EOF
   ssh_public_key_file = "$PWD/_debug/lab/id_ed25519.pub"
   allowed_ssh_cidrs   = ["$(curl -s https://api.ipify.org)/32"]
   EOF
   ```

3. Create the server. If Hetzner has no capacity for the server type, add for example
   `-var server_type=cpx62 -var location=fsn1`.

   ```shell
   set -a; . ./.env; set +a
   terraform -chdir=hack/lab init
   terraform -chdir=hack/lab apply -var-file=../../_debug/lab/lab.tfvars -state=../../_debug/lab/terraform.tfstate
   ```

4. Wait until cloud-init has installed the tools, then copy the repository to the server:

   ```shell
   ssh -i _debug/lab/id_ed25519 root@SERVER_IP 'until [ -f /var/lib/lab-ready ]; do sleep 5; done'
   rsync -a --exclude _debug --exclude .env -e "ssh -i _debug/lab/id_ed25519" ./ root@SERVER_IP:/root/work/crossplane-provider-superset/
   ```

On the server, `make superset.up` starts Superset, and `test/e2e/run.sh` tests the examples
(see `contributing/development.md`).

## Destroy the lab

```shell
set -a; . ./.env; set +a
terraform -chdir=hack/lab destroy -var-file=../../_debug/lab/lab.tfvars -state=../../_debug/lab/terraform.tfstate
```

The destroy removes the server, its SSH key and its firewall in Hetzner Cloud.
