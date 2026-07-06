# GHCR ARM64 Docker Image

This repository publishes an ARM64 image for iStoreOS/OpenWrt Docker:

```text
ghcr.io/neocj/vohive-test:arm64
ghcr.io/neocj/vohive-test:latest-arm64
```

The image is built by GitHub Actions from the current repository source:

- frontend from `web/`
- backend from the Go source at the repository root
- no `GH_PAT`
- no private repositories
- no prebuilt VoHive binaries

Use `compose.yaml` for deployment. It publishes only port `7575` on
`127.0.0.1` by default, mounts only `./config`, `./data`, and `./logs`, and
uses explicit device variables instead of mapping the whole `/dev` tree.
