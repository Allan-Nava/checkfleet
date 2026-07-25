# Minimal image for the checkfleet binary (built & injected by goreleaser).
# distroless static: no shell, non-root, includes CA certificates for TLS.
# Default command is the Prometheus exporter; mount a config and override args
# for one-shot checks, e.g.:
#   docker run -v $PWD/checkfleet.yml:/checkfleet.yml ghcr.io/allan-nava/checkfleet \
#     check all --config /checkfleet.yml
FROM gcr.io/distroless/static:nonroot
COPY checkfleet /usr/bin/checkfleet
EXPOSE 9876
ENTRYPOINT ["/usr/bin/checkfleet"]
CMD ["serve", "--listen", ":9876", "--config", "/checkfleet.yml"]
