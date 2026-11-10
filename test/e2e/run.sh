#!/usr/bin/env bash
# End-to-end test of the provider against a real Apache Superset.
#
# The script creates a kind cluster, installs the CRDs, and runs the provider binary outside
# the cluster. For each manifest that you name, it
#
#   1. applies the manifest and waits until every managed resource in it is Ready and Synced,
#   2. checks that the provider does not update them again after a poll (no update loop),
#   3. applies the manifest's update, if it has one (see below), and checks steps 1 and 2 again,
#   4. imports each managed resource: it deletes the managed resource with the Orphan deletion
#      policy and creates it again with only its external name, the way you take over an
#      existing object. The managed resource must become Ready without creating anything,
#   5. deletes the manifest and waits until the managed resources are gone.
#
# An update is a line in the manifest of the form
#   # e2e-update: <kind>/<name> <JSON merge patch>
# for example
#   # e2e-update: Role/analysts {"spec":{"forProvider":{"name":"Analysts (renamed)"}}}
#
# Usage:
#   test/e2e/run.sh examples/cluster/security/role.yaml examples/namespaced/chart/chart.yaml
#   test/e2e/run.sh            # every example in examples/cluster and examples/namespaced
#
# Superset and the credentials come from the environment, or from .env in the repository:
#   SUPERSET_HOST, SUPERSET_USERNAME and SUPERSET_PASSWORD (see example.env). The Superset of
#   `make superset.up` works with the values of example.env.
#
# Options (environment variables):
#   KEEP=1       keep the cluster and the provider running afterwards
#   CLUSTER=name use or create this kind cluster (default provider-superset-e2e)
#   TIMEOUT=5m   how long to wait for each manifest to become ready
#   POLL=20s     the provider's poll interval
#   IDLE=40s     how long to watch a ready manifest for update loops (0 turns the check off)
#   IMPORT=0     skip the import step
#   PROVIDER=... use this provider binary instead of building one
#   E2E_LOG=file copy the provider log to this file at the end
#
# Needs docker, kind, kubectl, jq and go.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
cluster=${CLUSTER:-provider-superset-e2e}
timeout=${TIMEOUT:-5m}
idle=${IDLE:-40s}
export KUBECONFIG="$work/kubeconfig"

if [[ -f "$root/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  . "$root/.env"
  set +a
fi
: "${SUPERSET_HOST:?set SUPERSET_HOST (or put it into .env)}"
: "${SUPERSET_USERNAME:?set SUPERSET_USERNAME}"
: "${SUPERSET_PASSWORD:?set SUPERSET_PASSWORD}"

log() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
fail() {
  printf '\033[31mFAIL: %s\033[0m\n' "$*"
  echo "--- provider log (last 40 lines)"
  tail -40 "$work/provider.log" | cut -c1-400 || true
  exit 1
}

cleanup() {
  local rc=$?
  if [[ -n "${E2E_LOG:-}" && -f "$work/provider.log" ]]; then
    cp "$work/provider.log" "$E2E_LOG"
  fi
  if [[ "${KEEP:-}" != 1 ]]; then
    # Delete whatever is left while the provider still runs, so that a failed run does not
    # leave objects in Superset.
    if [[ -n "${provider_pid:-}" ]] && kill -0 "$provider_pid" 2>/dev/null; then
      kubectl delete managed --all -A --wait=true --timeout=300s >/dev/null 2>&1 ||
        echo "WARNING: some managed resources were not deleted; check Superset for leftovers"
    fi
    [[ -n "${provider_pid:-}" ]] && kill "$provider_pid" 2>/dev/null || true
    kind delete cluster --name "$cluster" >/dev/null 2>&1 || true
    rm -rf "$work"
  else
    echo "kept: KUBECONFIG=$work/kubeconfig, provider log $work/provider.log, provider PID ${provider_pid:-none}"
  fi
  exit $rc
}
trap cleanup EXIT

if [[ $# -eq 0 ]]; then
  mapfile -t manifests < <(find "$root/examples/cluster" "$root/examples/namespaced" -name '*.yaml' \
    ! -path '*/providerconfig/*' ! -name 'install.yaml' | sort)
else
  manifests=("$@")
fi

log "building the provider"
provider=${PROVIDER:-}
if [[ -z "$provider" ]]; then
  provider="$work/provider"
  (cd "$root" && go build -o "$provider" ./cmd/provider)
fi

log "creating the kind cluster $cluster"
if ! kind get clusters 2>/dev/null | grep -qx "$cluster"; then
  kind create cluster --name "$cluster" --wait 180s >/dev/null
fi
kind get kubeconfig --name "$cluster" > "$KUBECONFIG"
kubectl apply --server-side -f "$root/package/crds" >/dev/null
kubectl wait crd --all --for=condition=Established --timeout=120s >/dev/null

log "starting the provider"
# The metrics server is off, so that two runs on one machine do not need the same port.
"$provider" --debug --no-leader-election --poll="${POLL:-20s}" --metrics-bind-address=0 > "$work/provider.log" 2>&1 &
provider_pid=$!
sleep 3
kill -0 "$provider_pid" 2>/dev/null || fail "the provider stopped right after it started"

log "creating the provider configurations"
creds=$(jq -n --arg host "$SUPERSET_HOST" --arg user "$SUPERSET_USERNAME" --arg pass "$SUPERSET_PASSWORD" \
  '{host: $host, username: $user, password: $pass}')
kubectl create namespace crossplane-system --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n crossplane-system create secret generic superset-creds --from-literal=credentials="$creds" \
  --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl apply -f - >/dev/null <<EOF
apiVersion: superset.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef: {namespace: crossplane-system, name: superset-creds, key: credentials}
---
apiVersion: superset.m.crossplane.io/v1beta1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef: {namespace: crossplane-system, name: superset-creds, key: credentials}
EOF

# managed_in prints "<resource>/<name> <namespace> <kind>" for each managed resource in a manifest.
managed_in() {
  kubectl apply --dry-run=client -f "$1" -o json |
    jq -r '(if .kind == "List" then .items[] else . end)
      | select(.apiVersion | test("superset(\\.m)?\\.crossplane\\.io/v1alpha1$"))
      | "\(.kind | ascii_downcase).\(.apiVersion | split("/")[0])/\(.metadata.name) \(.metadata.namespace // "-") \(.kind)"'
}

nsflag() { [[ "$1" != "-" ]] && echo "-n $1" || true; }

# wait_ready waits until a managed resource is Ready and Synced.
wait_ready() {
  local res=$1 ns=$2
  # shellcheck disable=SC2046
  if ! kubectl wait "$res" $(nsflag "$ns") --for=condition=Ready --timeout="$timeout" >/dev/null; then
    # shellcheck disable=SC2046
    kubectl get "$res" $(nsflag "$ns") -o jsonpath='{.status.conditions}' | jq . || true
    fail "$res did not become Ready"
  fi
  # shellcheck disable=SC2046
  if ! kubectl wait "$res" $(nsflag "$ns") --for=condition=Synced --timeout=120s >/dev/null; then
    # shellcheck disable=SC2046
    kubectl get "$res" $(nsflag "$ns") -o jsonpath='{.status.conditions}' | jq . || true
    fail "$res did not become Synced"
  fi
}

# wait_all waits until every managed resource of a manifest is Ready and Synced.
wait_all() {
  while read -r res ns _; do
    [[ -z "$res" ]] && continue
    wait_ready "$res" "$ns"
    echo "ready: $res"
  done < <(managed_in "$1")
}

# check_idle makes sure that the provider does not update a manifest's managed resources
# again after the next poll; otherwise an update loop exists.
check_idle() {
  [[ "$idle" == 0 ]] && return
  local lines
  lines=$(wc -l < "$work/provider.log")
  sleep "$idle"
  while read -r res _; do
    [[ -z "$res" ]] && continue
    local name=${res##*/}
    if tail -n +"$lines" "$work/provider.log" | grep -E "Async update starting|Updating the external resource|Successfully requested update of external resource" |
      grep -q "\"resourceName\": \"$name\"\|\"name\": \"$name\""; then
      fail "the provider updated $res again after it was ready (update loop)"
    fi
  done < <(managed_in "$1")
  echo "no update loop"
}

# apply_updates applies the "# e2e-update:" lines of a manifest.
apply_updates() {
  local m=$1 any=1
  while read -r target patch; do
    [[ -z "$target" ]] && continue
    any=0
    local kind=${target%%/*} name=${target#*/} res ns
    read -r res ns _ < <(managed_in "$m" | awk -v k="$kind" -v n="$name" '$3 == k && $1 ~ ("/" n "$")')
    [[ -n "$res" ]] || fail "$m: the update names $target, which the manifest does not have"
    # shellcheck disable=SC2046
    kubectl patch "$res" $(nsflag "$ns") --type merge -p "$patch" >/dev/null || fail "cannot patch $res"
    updated+=("$res $ns $patch")
    echo "updated: $res"
  done < <(sed -n 's/^# e2e-update: *\([^ ]*\) \(.*\)$/\1 \2/p' "$m")
  return $any
}

# check_updates makes sure that each updated value of spec.forProvider reached Superset: the
# provider reads it back into status.atProvider. Only top-level values that are not
# references or secrets are compared.
check_updates() {
  local entry res ns patch observed
  for entry in "${updated[@]}"; do
    read -r res ns patch <<<"$entry"
    local ok=1
    for _ in $(seq 1 24); do
      # shellcheck disable=SC2046
      observed=$(kubectl get "$res" $(nsflag "$ns") -o json) || fail "cannot read $res"
      # An object in the patch must be a subset of the observed object, because a merge
      # patch can change some keys of a map (such as the files of an import) only, and the
      # observed elements of a list can have computed fields (such as the ids of permissions).
      # Lists are compared without their order, because Superset may return a set in another
      # order.
      if jq -e --argjson patch "$patch" '
          def sub($a; $b): if ($a | type) == "object"
            then ($b | type) == "object" and ([$a | to_entries[] | . as $e | sub($e.value; $b[$e.key])] | all)
            elif ($a | type) == "array"
            then ($b | type) == "array" and ($a | length) == ($b | length)
              and ([$a[] as $x | any($b[]; sub($x; .))] | all)
              and ([$b[] as $y | any($a[]; sub(.; $y))] | all)
            else $a == $b end;
          . as $mr | ($patch.spec.forProvider // {}) | to_entries
          | map(select((.key | test("(Ref|Refs|Selector|SecretRef)$") | not)))
          | all(. as $e | sub($e.value; $mr.status.atProvider[$e.key]))' <<<"$observed" >/dev/null; then
        ok=0
        break
      fi
      sleep 5
    done
    if [[ $ok != 0 ]]; then
      jq '.status.atProvider' <<<"$observed"
      fail "the update of $res did not reach Superset: status.atProvider differs from $patch"
    fi
    echo "verified: $res"
  done
}

# import_all takes every managed resource of a manifest over again by its external name. It
# removes the managed resource without deleting its object (cluster-scoped kinds use the
# deletion policy Orphan; namespaced kinds have no deletion policy, so the management policies
# leave out Delete), creates it again with only the external-name annotation, and checks that
# it becomes Ready with the same external name. An import kind imports its bundle again, which
# overwrites the same objects and gives the same hash.
import_all() {
  while read -r res ns _; do
    [[ -z "$res" ]] && continue
    local ext obj orphan
    # shellcheck disable=SC2046
    ext=$(kubectl get "$res" $(nsflag "$ns") -o jsonpath='{.metadata.annotations.crossplane\.io/external-name}')
    [[ -n "$ext" ]] || fail "$res has no external name"
    # shellcheck disable=SC2046
    obj=$(kubectl get "$res" $(nsflag "$ns") -o json | jq 'del(.status, .metadata.uid, .metadata.resourceVersion,
      .metadata.creationTimestamp, .metadata.generation, .metadata.managedFields, .metadata.finalizers)
      | .metadata.annotations |= with_entries(select(.key == "crossplane.io/external-name"))')
    if [[ "$ns" == "-" ]]; then
      orphan='{"spec":{"deletionPolicy":"Orphan"}}'
    else
      orphan='{"spec":{"managementPolicies":["Observe","Create","Update","LateInitialize"]}}'
    fi
    # shellcheck disable=SC2046
    kubectl patch "$res" $(nsflag "$ns") --type merge -p "$orphan" >/dev/null || fail "cannot orphan $res"
    # shellcheck disable=SC2046
    kubectl delete "$res" $(nsflag "$ns") --wait=true --timeout=120s >/dev/null || fail "cannot delete $res"
    kubectl create -f - <<<"$obj" >/dev/null || fail "cannot create $res again"
    wait_ready "$res" "$ns"
    local again
    # shellcheck disable=SC2046
    again=$(kubectl get "$res" $(nsflag "$ns") -o jsonpath='{.metadata.annotations.crossplane\.io/external-name}')
    [[ "$again" == "$ext" ]] || fail "$res was created again instead of imported: external name $ext became $again"
    echo "imported: $res ($ext)"
  done < <(managed_in "$1")
}

# test_manifest runs every step for one manifest. It runs in a subshell, so that a failure
# ends only the test of this manifest.
test_manifest() {
  local m=$1
  [[ -n "$(managed_in "$m")" ]] || fail "$m holds no managed resource of this provider"
  kubectl apply -f "$m" >/dev/null || fail "kubectl apply -f $m"
  wait_all "$m"
  check_idle "$m"

  updated=()
  if apply_updates "$m"; then
    # Give the provider a moment to see the change before waiting for the conditions.
    sleep 5
    wait_all "$m"
    check_updates
    check_idle "$m"
  fi

  if [[ "${IMPORT:-1}" != 0 ]]; then
    import_all "$m"
    check_idle "$m"
  fi

  kubectl delete -f "$m" --wait=true --timeout="$timeout" >/dev/null || fail "$m was not deleted in time"
}

passed=0
failed=()
for m in "${manifests[@]}"; do
  if ! kill -0 "$provider_pid" 2>/dev/null; then
    failed+=("$m (the provider stopped)")
    continue
  fi
  log "testing $m"
  # The subshell turns errexit on again: inside "if" or "||", bash would ignore it.
  set +e
  (
    set -e
    test_manifest "$m"
  )
  rc=$?
  set -e
  if [[ $rc == 0 ]]; then
    passed=$((passed + 1))
  else
    failed+=("$m")
    # Remove what the failed test left, so that the next manifests start clean.
    kubectl delete -f "$m" --ignore-not-found --wait=true --timeout="$timeout" >/dev/null 2>&1 ||
      echo "WARNING: $m was not deleted after its failure; check Superset for leftovers"
  fi
done

# The update-loop check reads the provider log. Make sure that the log has the format that
# the check expects, so that the check cannot pass by accident.
if [[ "$idle" != 0 ]] && ! grep -qE '"(resourceName|name)": "' "$work/provider.log"; then
  fail "the provider log has no \"resourceName\" or \"name\" fields, so the update-loop check did not work"
fi

log "passed: $passed manifest(s), failed: ${#failed[@]}"
if [[ ${#failed[@]} -gt 0 ]]; then
  printf '  failed: %s\n' "${failed[@]}"
  exit 1
fi
