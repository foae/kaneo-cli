FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/kaneo-cli /usr/local/bin/kaneo-cli
COPY LICENSE /usr/share/licenses/kaneo-cli/LICENSE

USER 65532:65532
ENV HOME=/home/nonroot
ENTRYPOINT ["/usr/local/bin/kaneo-cli"]
