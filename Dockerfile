# Overlook's image: the static binary GoReleaser built for the platform, and
# the CA certificates it checks HTTPS sources with, and nothing else.
#
# It has no volume, since Overlook remembers nothing (ADR 0004). Mount the
# folder that holds overlook.yaml, and icons/ and custom.css if you have them,
# read-only at /etc/overlook. TZ says when today starts: the zone database is
# inside the binary.

# Only the certificates are taken from it, so it runs nothing and one serves
# every platform.
FROM --platform=$BUILDPLATFORM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS certs

FROM scratch
ARG TARGETPLATFORM
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY $TARGETPLATFORM/overlook /overlook
# overlook.yaml is read from here, by default.
WORKDIR /etc/overlook
# A user with no rights to anything, as distroless's nonroot is.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/overlook"]
