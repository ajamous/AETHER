FROM golang:1.26-alpine AS build
WORKDIR /src
COPY services/smdp-plus/go.mod ./services/smdp-plus/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY services/certmgr/go.mod ./services/certmgr/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/
COPY pkg/hsmclient/go.mod ./pkg/hsmclient/
COPY pkg/certmgrclient/go.mod ./pkg/certmgrclient/
COPY pkg/saip/go.mod ./pkg/saip/

RUN cd services/smdp-plus && go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

RUN cd services/smdp-plus && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/smdp-plus ./cmd/smdp-plus

FROM alpine:3.23
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/smdp-plus /usr/local/bin/smdp-plus
USER aether
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/smdp-plus"]
CMD ["--listen=:8443"]
