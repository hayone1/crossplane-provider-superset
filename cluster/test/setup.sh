#!/usr/bin/env bash
# Prepares a cluster for uptest (make e2e). UPTEST_CLOUD_CREDENTIALS holds the credentials
# JSON object that a ProviderConfig Secret holds, for example
# {"host": "http://superset:8088", "username": "admin", "password": "..."}.
set -aeuo pipefail

echo "Creating the credentials secret..."
${KUBECTL} -n crossplane-system create secret generic superset-creds \
  --from-literal=credentials="${UPTEST_CLOUD_CREDENTIALS}" --dry-run=client -o yaml | ${KUBECTL} apply -f -

echo "Waiting until the provider is healthy..."
${KUBECTL} wait provider.pkg --all --for condition=Healthy --timeout 5m

echo "Waiting for all pods to come online..."
${KUBECTL} -n crossplane-system wait --for=condition=Available deployment --all --timeout=5m

echo "Creating the default ProviderConfig and ClusterProviderConfig..."
cat <<EOF | ${KUBECTL} apply -f -
apiVersion: superset.crossplane.io/v1beta1
kind: ProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef:
      name: superset-creds
      namespace: crossplane-system
      key: credentials
---
apiVersion: superset.m.crossplane.io/v1beta1
kind: ClusterProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef:
      name: superset-creds
      namespace: crossplane-system
      key: credentials
EOF
