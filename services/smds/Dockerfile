FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/smds/go.mod ./services/smds/
COPY services/audit/go.mod ./services/audit/
COPY services/profile-builder/go.mod services/profile-builder/go.sum ./services/profile-builder/
COPY services/gateway/go.mod ./services/gateway/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY services/certmgr/go.mod ./services/certmgr/
COPY services/smdp-plus/go.mod ./services/smdp-plus/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/

RUN cd services/smds && go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

RUN cd services/smds && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/smds ./cmd/smds

FROM alpine:3.19
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/smds /usr/local/bin/smds
USER aether
EXPOSE 8448
ENTRYPOINT ["/usr/local/bin/smds"]
CMD ["--listen=:8448"]
