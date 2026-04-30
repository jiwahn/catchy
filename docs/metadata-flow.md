# OCI Metadata Flow

OCI images and runtime bundles can carry metadata in several different places. Those places are related, but they are not interchangeable. When debugging hook behavior, it is common to see metadata in an image tool and then not find the same data in the final OCI runtime `config.json`.

This document explains the common metadata locations and where propagation can break down, especially in containerd plus low-level runtime workflows such as `runc`, `youki`, and `urunc`.

## Image Index Annotations

Image index annotations live on a multi-platform image index. They may describe the image reference as a whole, such as a logical application, release, or platform set.

For multi-architecture images, selecting a platform matters. Top-level index annotations are not the same as annotations on the platform-specific image manifest. A tool may show index-level metadata even though the runtime later resolves a specific platform manifest.

## Image Manifest Annotations

Image manifest annotations live on an image manifest or manifest descriptor. Tools such as `crane` and `skopeo` can show these annotations directly.

Container runtimes often do not propagate image manifest annotations into the OCI runtime spec. Depending on the runtime path, annotations that are visible in registry or image tooling may never appear in the final bundle `config.json`.

## Image Config Labels

Image config labels live in the image config under `config.Labels`. A Dockerfile `LABEL` instruction usually ends up here.

containerd commonly exposes image config labels as container metadata labels. These labels are generally more likely to survive into container-level metadata than manifest annotations, although that still does not guarantee they become OCI runtime spec annotations.

## Container Metadata Labels

containerd containers can have labels. These are metadata attached to the container object in containerd.

Container metadata labels are not the same as OCI runtime spec annotations. A label being visible in containerd metadata does not automatically mean it appears in `config.json` under `annotations`.

## OCI Runtime Spec Annotations

OCI runtime spec annotations live in the bundle `config.json` under `annotations`.

Hooks and low-level runtimes may read these annotations. They are optional, and container runtimes do not necessarily populate them from image manifest annotations. Some metadata may stay at the image or container metadata layer by design.

## Conceptual Flow

This is a conceptual model. Exact behavior depends on the image tool, container runtime, snapshotter, shim, and low-level runtime.

```text
image index annotations
|
v
image manifest annotations      image config labels
|                               |
|                               v
|                        container metadata labels
|                               |
v                               v
usually not                  may be available to
propagated                   higher-level runtime logic

runtime spec annotations
|
v
OCI runtime / hooks
```

The important point is that there is no automatic universal path from image annotations to runtime spec annotations.

## Common Propagation Gap

A common failure mode looks like this:

1. A user builds or publishes an image with manifest annotations.
2. The annotations are visible with `crane` or `skopeo`.
3. The container starts through containerd.
4. The final OCI runtime `config.json` has no corresponding annotations.
5. A low-level runtime or hook expecting those annotations cannot find them.

For example, an image might contain urunc-style metadata keys such as:

```text
com.urunc.unikernel.binary
com.urunc.unikernel.hypervisor
com.urunc.unikernel.unikernelType
```

Those keys may be visible as manifest annotations, but that does not necessarily mean they will appear as runtime spec annotations. In many containerd workflows, image config labels are more likely to be visible as container metadata than manifest annotations are.

## How Catchy Helps

`catchy trace-metadata <image>` shows image manifest annotations, image config labels, the source tool used (`crane`, `skopeo`, or `docker`), the manifest media type, and observations about likely propagation gaps.

`catchy inspect <bundle>` shows OCI runtime hook definitions in a bundle. It can be used alongside direct inspection of `config.json` annotations.

`catchy check <bundle>` validates hook paths and obvious setup mistakes before running the bundle.

`catchy bundle-path`, `catchy inspect-containerd`, and `catchy check-containerd` help locate and inspect containerd runtime v2 bundles when you know the namespace and container ID.

Current limitations:

* `catchy` does not yet compare image metadata directly against runtime spec annotations.
* `catchy` does not yet query the containerd API for image or container metadata.
* `trace-metadata` is read-only.

## Example Workflow

Step 1: inspect image metadata.

```sh
catchy trace-metadata harbor.example.com/urunc/nginx:latest
```

Step 2: find the containerd runtime v2 bundle.

```sh
sudo catchy bundle-path --namespace default --id test
```

For Kubernetes on containerd, the namespace is often `k8s.io`.

Step 3: inspect the resolved bundle.

```sh
sudo catchy inspect-containerd --namespace default --id test
```

Step 4: manually compare the image metadata from `trace-metadata` with the bundle `config.json` annotations.

```sh
sudo jq '.annotations' /run/containerd/io.containerd.runtime.v2.task/default/test/config.json
```

## What This Does Not Prove

Seeing manifest annotations does not prove they reached the runtime.

Seeing config labels does not prove they became runtime spec annotations.

Missing runtime annotations does not necessarily mean the image metadata is absent. It may have been stored elsewhere, such as image config labels or container metadata labels, or not propagated by design.

Docker fallback in `trace-metadata` may not show remote manifest annotations. Prefer `crane` or `skopeo` when debugging registry-side metadata.

## Future Work

Possible future commands:

```text
catchy compare-metadata --image <image> --bundle <bundle>
catchy compare-metadata-containerd --image <image> --namespace <ns> --id <id>
catchy trace-metadata --platform linux/amd64 <image>
```

Other likely improvements:

* Read-only containerd API lookup for image config labels and container labels.
* Platform-specific manifest selection for multi-arch images.
* A direct report showing image metadata, container metadata, and runtime spec annotations side by side.
