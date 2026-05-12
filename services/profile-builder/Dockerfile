FROM golang:1.26-alpine AS build
WORKDIR /src
COPY services/profile-builder/go.mod services/profile-builder/go.sum ./services/profile-builder/
COPY services/hsm-broker/go.mod services/hsm-broker/go.sum ./services/hsm-broker/
COPY services/certmgr/go.mod ./services/certmgr/
COPY services/smdp-plus/go.mod ./services/smdp-plus/
COPY pkg/crypto/go.mod ./pkg/crypto/
COPY pkg/asn1/sgp22/go/go.mod ./pkg/asn1/sgp22/go/
COPY pkg/saip/go.mod ./pkg/saip/

RUN cd services/profile-builder && go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

RUN cd services/profile-builder && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
      -o /out/profile-builder ./cmd/profile-builder

FROM alpine:3.23
RUN addgroup -S aether && adduser -S aether -G aether
COPY --from=build /out/profile-builder /usr/local/bin/profile-builder
COPY --from=build /src/services/profile-builder/templates /etc/aether/profile-builder/templates
USER aether
EXPOSE 8446
ENTRYPOINT ["/usr/local/bin/profile-builder"]
CMD ["--listen=:8446", "--template-dir=/etc/aether/profile-builder/templates"]
