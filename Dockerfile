# Imagen OCI de Gremio (ADR-007). La arma GoReleaser (dockers_v2) con el binario ya compilado de cada
# plataforma, así que no compila nada ni necesita emulación. Base sin shell y usuario no root (65532).
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/gremio /usr/local/bin/gremio
COPY LICENSE /usr/share/doc/gremio/
COPY third_party_licenses /usr/share/doc/gremio/third_party_licenses
ENV GREMIO_DATA_DIR=/data \
    GREMIO_ADDR=0.0.0.0:8080
WORKDIR /data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["gremio", "healthcheck"]
ENTRYPOINT ["gremio"]
CMD ["serve"]
