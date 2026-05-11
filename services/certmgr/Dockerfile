FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/certmgr/go.mod ./services/certmgr/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/

RUN cd services/certmgr && go mod download

COPY pkg/ ./pkg/
COPY services/certmgr/ ./services/certmgr/

RUN cd services/certmgr && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/certmgr ./cmd/certmgr

FROM alpine:3.23
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/certmgr /usr/local/bin/certmgr
USER aether
EXPOSE 8444
ENTRYPOINT ["/usr/local/bin/certmgr"]
CMD ["--mode=lab", "--listen=:8444"]
