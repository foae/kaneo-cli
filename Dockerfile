FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/kaneo-cli /usr/local/bin/kaneo-cli
COPY LICENSE /usr/share/licenses/kaneo-cli/LICENSE

USER 65532:65532
ENV HOME=/home/nonroot
ENTRYPOINT ["/usr/local/bin/kaneo-cli"]
