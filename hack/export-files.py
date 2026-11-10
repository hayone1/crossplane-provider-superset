#!/usr/bin/env python3
"""Writes an import managed resource for a Superset export.

Superset exports dashboards, charts and datasets as a zip file. This script reads such a zip file
(or the folder that you unpacked it into) and prints a manifest whose spec.forProvider.files holds
every YAML file of the export, keyed by its path in the export. metadata.yaml is left out, because
the provider writes its own.

Usage:
    python3 hack/export-files.py EXPORT [--kind DashboardImport] [--name NAME]
        [--namespace NAMESPACE] [--password-secret SECRET] > import.yaml

EXPORT is a zip file or a folder. --kind is DashboardImport (the default), ChartImport,
DatasetImport or AssetsImport. Without --namespace, the manifest is cluster-scoped. With
--password-secret, the manifest reads the database passwords from that Secret, whose keys are the
file names of the databases, such as warehouse.yaml.

Needs Python 3 with PyYAML.
"""
import argparse
import pathlib
import sys
import zipfile

import yaml

KINDS = {"DashboardImport", "ChartImport", "DatasetImport", "AssetsImport"}

# The folders of the export layout.
LAYOUT = {"dashboards", "charts", "datasets", "databases", "queries"}


class Literal(str):
    """A string that YAML writes as a literal block."""


def literal(dumper, data):
    return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")


yaml.SafeDumper.add_representer(Literal, literal)


def read_export(source: pathlib.Path) -> dict:
    """Returns the files of the export keyed by their path below the export's top folder."""
    entries = {}
    if source.is_dir():
        for path in sorted(source.rglob("*.yaml")):
            entries[path.relative_to(source).as_posix()] = path.read_text()
    else:
        with zipfile.ZipFile(source) as archive:
            for name in sorted(archive.namelist()):
                if name.endswith(".yaml"):
                    entries[name] = archive.read(name).decode()
    # An export holds one top folder, such as dashboard_export_20260101T000000/. The paths in the
    # manifest start below it.
    tops = {name.split("/", 1)[0] for name in entries}
    if len(tops) == 1 and all("/" in name for name in entries) and not tops & LAYOUT:
        entries = {name.split("/", 1)[1]: content for name, content in entries.items()}
    return {name: content for name, content in entries.items() if name != "metadata.yaml"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("export", type=pathlib.Path)
    parser.add_argument("--kind", default="DashboardImport", choices=sorted(KINDS))
    parser.add_argument("--name", default="imported")
    parser.add_argument("--namespace")
    parser.add_argument("--password-secret")
    args = parser.parse_args()

    files = read_export(args.export)
    if not files:
        print(f"{args.export} holds no YAML files", file=sys.stderr)
        return 1

    group = "assets.superset.m.crossplane.io" if args.namespace else "assets.superset.crossplane.io"
    metadata = {"name": args.name}
    if args.namespace:
        metadata["namespace"] = args.namespace
    for_provider = {"files": {name: Literal(content) for name, content in files.items()}}
    if args.password_secret:
        ref = {"name": args.password_secret}
        if not args.namespace:
            ref["namespace"] = "crossplane-system"
        for_provider["databasePasswordsSecretRef"] = ref
    manifest = {
        "apiVersion": f"{group}/v1alpha1",
        "kind": args.kind,
        "metadata": metadata,
        "spec": {"forProvider": for_provider},
    }
    yaml.safe_dump(manifest, sys.stdout, sort_keys=False, width=1000)
    return 0


if __name__ == "__main__":
    sys.exit(main())
