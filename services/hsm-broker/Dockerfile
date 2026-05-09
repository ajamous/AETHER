# Aether HSM broker image.
#
# Two stages: a Go build, then a minimal runtime image with only what
# the broker needs at runtime — the SoftHSM libraries are added in the
# lab compose layer rather than baked in here, since production
# deployments swap in a different PKCS#11 library.

FROM golang:1.22-alpine AS build
WORKDIR /src

# Copy the workspace pieces this service depends on.
COPY go.work ./
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/

# Pre-fetch deps for caching.
RUN cd services/hsm-broker && go mod download

# Now the source.
COPY pkg/ ./pkg/
COPY services/hsm-broker/ ./services/hsm-broker/

RUN cd services/hsm-broker && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/hsm-broker ./cmd/hsm-broker

FROM alpine:3.19
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/hsm-broker /usr/local/bin/hsm-broker
USER aether
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/hsm-broker"]
CMD ["--backend=memory", "--listen=:8443"]
