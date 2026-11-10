#!/usr/bin/env python3
"""Writes the import examples in examples/cluster/assets from the export in test/superset/export.

test/superset/export holds a dashboard export of Superset: one dashboard with one chart, its
dataset and its database. Each import example puts the files that its kind needs into
spec.forProvider.files, so that the example is complete in one manifest. Run it after you
change the export:

    python3 hack/import-examples.py
"""
import json
import pathlib

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
EXPORT = ROOT / "test" / "superset" / "export"
DST = ROOT / "examples" / "cluster" / "assets"

# The password of the database in the export, which Superset leaves out of an export.
SECRET = """apiVersion: v1
kind: Secret
metadata:
  name: {name}
  namespace: crossplane-system
type: Opaque
stringData:
  # The key is the file name of the database in the bundle (databases/<file>).
  {database}: examples
"""

KINDS = {
    "dashboardimport": ("DashboardImport", ["dashboards", "charts", "datasets", "databases"],
                        "This example imports a dashboard with its chart, dataset and database. Deleting the\n"
                        "# managed resource deletes the dashboard. The chart, the dataset and the database stay."),
    "chartimport": ("ChartImport", ["charts", "datasets", "databases"],
                    "This example imports a chart with its dataset and database. Deleting the managed\n"
                    "# resource deletes the chart. The dataset and the database stay."),
    "datasetimport": ("DatasetImport", ["datasets", "databases"],
                      "This example imports a dataset with its database. Deleting the managed resource\n"
                      "# deletes the dataset. The database stays."),
    "assetsimport": ("AssetsImport", ["dashboards", "charts", "datasets", "databases"],
                     "This example imports a whole export at once: the dashboard, its chart, its dataset\n"
                     "# and its database. Deleting the managed resource deletes the dashboard, the chart and\n"
                     "# the dataset. The database stays."),
}


class Literal(str):
    pass


def literal(dumper, data):
    return dumper.represent_scalar("tag:yaml.org,2002:str", data, style="|")


yaml.SafeDumper.add_representer(Literal, literal)


def main():
    files = {str(p.relative_to(EXPORT)): p.read_text() for p in sorted(EXPORT.rglob("*.yaml"))
             if p.name != "metadata.yaml"}
    database = next(pathlib.PurePath(f).name for f in files if f.startswith("databases/"))
    DST.mkdir(parents=True, exist_ok=True)
    for name, (kind, dirs, comment) in KINDS.items():
        chosen = {f: Literal(c) for f, c in files.items() if f.split("/")[0] in dirs}
        # The update changes the description of the first object of the kind's own directory,
        # so the provider imports the bundle again.
        own = sorted(f for f in chosen if f.startswith(dirs[0] + "/"))[0]
        changed = chosen[own].replace("description: null", "description: Changed by the end-to-end test", 1)
        update = {"spec": {"forProvider": {"files": {own: changed}}}}
        mr = {
            "apiVersion": "assets.superset.crossplane.io/v1alpha1",
            "kind": kind,
            "metadata": {"name": f"sales-{name}"},
            "spec": {"forProvider": {
                "files": chosen,
                "databasePasswordsSecretRef": {"name": f"sales-{name}-db", "namespace": "crossplane-system"},
            }},
        }
        text = f"# {comment}\n"
        text += f"# e2e-update: {kind}/sales-{name} {json.dumps(update, separators=(',', ':'))}\n"
        text += SECRET.format(name=f"sales-{name}-db", database=database)
        text += "---\n" + yaml.safe_dump(mr, sort_keys=False, width=1000)
        (DST / f"{name}.yaml").write_text(text)
        print(f"wrote {DST / name}.yaml with {len(chosen)} files")


if __name__ == "__main__":
    main()
