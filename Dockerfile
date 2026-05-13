# Distroless base — tiny + no shell, suitable for daemon use.
# goreleaser builds the binary outside Docker and COPYs it in.
FROM gcr.io/distroless/static-debian12:nonroot

COPY yomiro /usr/local/bin/yomiro

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/yomiro"]
CMD ["gw", "run"]
