FROM golang:1.27-alpine AS build
WORKDIR /src
COPY services/gateway/go.mod ./services/gateway/
COPY services/profile-builder/go.mod services/profile-builder/go.sum ./services/profile-builder/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY services/certmgr/go.mod ./services/certmgr/
COPY services/smdp-plus/go.mod ./services/smdp-plus/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/

RUN cd services/gateway && go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

RUN cd services/gateway && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/gateway ./cmd/gateway

FROM alpine:3.23
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/gateway /usr/local/bin/gateway
USER aether
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gateway"]
CMD ["--listen=:8080"]
