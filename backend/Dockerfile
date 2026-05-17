FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -ldflags "-s -w \
    -X github.com/qorven/qorven/cmd.Version=${VERSION} \
    -X github.com/qorven/qorven/cmd.Commit=${COMMIT}" \
    -o /qorven .

FROM alpine:3.21
# wget is used by the compose-level healthcheck + Dockerfile HEALTHCHECK.
# ca-certificates for TLS; tzdata for correct timestamps.
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=builder /qorven /usr/local/bin/qorven
COPY scripts/install.sh /usr/local/share/qorven/install.sh
EXPOSE 4200
# Point at /livez (not /health) — matches the compose healthcheck and
# the k8s convention. /livez is the cheapest "is the process alive"
# probe; it deliberately skips DB checks so a DB blip doesn't trigger
# a container restart.
HEALTHCHECK --interval=15s --timeout=3s --start-period=30s --retries=3 \
  CMD wget -qO- http://127.0.0.1:4200/livez || exit 1
ENTRYPOINT ["qorven"]
CMD ["start"]
