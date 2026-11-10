# Usage

`icon.svg` is the package icon that the Upbound Marketplace shows. It shows a bar chart inside a
reconcile loop. It is original artwork for this project, under the same Apache 2.0 license as
the rest of the repository. It contains no Apache Superset marks, logos or brand colors.

Do not replace it with the Apache Superset logo or anything that looks like it. The logo is a
trademark of the Apache Software Foundation, and using it could suggest that the foundation
endorses this project.

The image must:

- be named `icon.svg`,
- be square, and
- be a valid [SVG](https://www.svgviewer.dev/) file.

The release workflow attaches `extensions/` to every published package with
`up alpha xpkg append`. The `meta.crossplane.io/iconURI` annotation in
`package/crossplane.yaml` points at this file as a fallback.
