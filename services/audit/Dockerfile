FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/audit/go.mod ./services/audit/
COPY services/profile-builder/go.mod services/profile-builder/go.sum ./services/profile-builder/
COPY services/gateway/go.mod ./services/gateway/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY services/certmgr/go.mod ./services/certmgr/
COPY services/smdp-plus/go.mod ./services/smdp-plus/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/

RUN cd services/audit && go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

RUN cd services/audit && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/audit ./cmd/audit

FROM alpine:3.19
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/audit /usr/local/bin/audit
USER aether
EXPOSE 8447
ENTRYPOINT ["/usr/local/bin/audit"]
CMD ["--listen=:8447"]
