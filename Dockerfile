# Distroless base — tiny + no shell, suitable for daemon use.
# goreleaser builds the binary outside Docker and COPYs it in.
# dockers_v2 stages per-arch binaries under linux/${TARGETARCH}/ in the
# build context, so we pick the matching one via the buildx-provided
# TARGETARCH arg.
FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETARCH
COPY linux/${TARGETARCH}/yomiro /usr/local/bin/yomiro

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/yomiro"]
CMD ["gw", "run"]
