FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/eim/go.mod services/eim/go.sum ./services/eim/
COPY services/smds/go.mod services/smds/go.sum ./services/smds/
COPY services/audit/go.mod services/audit/go.sum ./services/audit/
COPY services/profile-builder/go.mod services/profile-builder/go.sum ./services/profile-builder/
COPY services/gateway/go.mod ./services/gateway/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY services/certmgr/go.mod ./services/certmgr/
COPY services/smdp-plus/go.mod services/smdp-plus/go.sum ./services/smdp-plus/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/
COPY pkg/hsmclient/go.mod ./pkg/hsmclient/
COPY pkg/certmgrclient/go.mod ./pkg/certmgrclient/

RUN cd services/eim && go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

RUN cd services/eim && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/eim ./cmd/eim

FROM alpine:3.19
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/eim /usr/local/bin/eim
USER aether
EXPOSE 8449
ENTRYPOINT ["/usr/local/bin/eim"]
CMD ["--listen=:8449"]
