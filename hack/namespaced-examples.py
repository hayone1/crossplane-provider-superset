#!/usr/bin/env python3
"""Writes examples/namespaced from examples/cluster.

The namespaced managed resources (*.superset.m.crossplane.io) live in a namespace, and they read
Secrets from their own namespace. Their secret references and connection secrets therefore have
no namespace field. This script turns each cluster-scoped example into a namespaced one:

* the API group gets the ".m" part,
* every object gets "namespace: default",
* Secrets move from crossplane-system to default,
* the namespace fields of secret references and connection secrets go away, and
* a providerConfigRef gets "kind: ClusterProviderConfig". A namespaced providerConfigRef
  needs both kind and name. The default applies only when providerConfigRef is missing.

Run it after you change an example in examples/cluster:

    python3 hack/namespaced-examples.py
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
SRC = ROOT / "examples" / "cluster"
DST = ROOT / "examples" / "namespaced"
NAMESPACE = "default"

API = re.compile(r"^apiVersion: ([a-z0-9]+)\.superset\.crossplane\.io/")


def convert(text: str) -> str:
    out = []
    in_metadata = False
    for line in text.splitlines():
        line = API.sub(r"apiVersion: \1.superset.m.crossplane.io/", line)
        if line == "metadata:":
            in_metadata = True
            out.append(line)
            out.append(f"  namespace: {NAMESPACE}")
            continue
        if in_metadata:
            if line.startswith("  namespace:"):
                # The namespace line was added right after "metadata:".
                continue
            if not line.startswith("  "):
                in_metadata = False
        # Namespace fields of secret references and connection secrets.
        if re.match(r"^\s{4,}namespace: crossplane-system$", line):
            continue
        out.append(line)
        m = re.match(r"^(\s*)providerConfigRef:$", line)
        if m:
            out.append(f"{m.group(1)}  kind: ClusterProviderConfig")
    return "\n".join(out) + "\n"


def main() -> int:
    # Remove the old copies first, so that a deleted cluster example leaves no namespaced copy.
    for old in DST.rglob("*.yaml"):
        if old.relative_to(DST).parts[0] != "providerconfig":
            old.unlink()
    for src in sorted(SRC.rglob("*.yaml")):
        rel = src.relative_to(SRC)
        if rel.parts[0] == "providerconfig":
            continue
        dst = DST / rel
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_text(convert(src.read_text()))
    return 0


if __name__ == "__main__":
    sys.exit(main())
