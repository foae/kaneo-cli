# Local acceptance only: upstream MinIO now distributes this release as source.
FROM golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195 AS builder
ENV CGO_ENABLED=0 GOTOOLCHAIN=local

FROM builder AS minio
WORKDIR /src/minio
# RELEASE.2025-10-15T17-29-55Z
RUN git init && git remote add origin https://github.com/minio/minio.git \
    && git fetch --depth 1 origin 9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a \
    && git checkout --detach FETCH_HEAD \
    && test "$(git rev-parse HEAD)" = 9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a
RUN flags="$(MINIO_RELEASE=RELEASE go run buildscripts/gen-ldflags.go 2025-10-15T17:29:55Z)" \
    && go build -tags kqueue -trimpath -ldflags "$flags" -o /out/minio . \
    && mkdir /data

FROM builder AS mc
WORKDIR /src/mc
# RELEASE.2025-08-13T08-35-41Z; needed by the bucket-creation acceptance command.
RUN git init && git remote add origin https://github.com/minio/mc.git \
    && git fetch --depth 1 origin 7394ce0dd2a80935aded936b09fa12cbb3cb8096 \
    && git checkout --detach FETCH_HEAD \
    && test "$(git rev-parse HEAD)" = 7394ce0dd2a80935aded936b09fa12cbb3cb8096
RUN flags="$(MC_RELEASE=RELEASE go run buildscripts/gen-ldflags.go 2025-08-13T08:35:41Z)" \
    && go build -tags kqueue -trimpath -ldflags "$flags" -o /out/mc .

FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
LABEL org.opencontainers.image.source="https://github.com/minio/minio" \
      org.opencontainers.image.revision="9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a" \
      org.opencontainers.image.version="RELEASE.2025-10-15T17-29-55Z" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later"
COPY --from=minio /out/minio /usr/local/bin/minio
COPY --from=mc /out/mc /usr/local/bin/mc
COPY --from=minio /src/minio/LICENSE /usr/share/licenses/minio/LICENSE
COPY --from=mc /src/mc/LICENSE /usr/share/licenses/mc/LICENSE
COPY --from=minio --chown=65532:65532 /data /data
USER 65532:65532
ENV HOME=/home/nonroot
ENTRYPOINT ["/usr/local/bin/minio"]
