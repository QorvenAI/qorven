# Qorven — single-binary Docker image.
#
# Multi-stage build:
#   1. web   — Next.js static export (QORVEN_STATIC=1)
#   2. backend — Go build with the web/out tree copied into go:embed
#                before compiling
#   3. runtime — Alpine, just the binary + ca-certs
#
# Result is one container image (~160MB) that ships the web UI and
# the API in one process. Pair with the pgvector/pgvector image for
# the database (see docker-compose.yml).

# ───────── web ─────────
FROM node:22-alpine AS web
WORKDIR /src/web
# pnpm is our package manager. Enabling corepack avoids a global npm
# install and pins to whatever pnpm version the lockfile expects.
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# QORVEN_STATIC=1 flips next.config.ts into output:'export' mode so
# web/out holds plain HTML/JS/CSS that the Go FileServer can serve.
ENV QORVEN_STATIC=1
RUN pnpm build

# ───────── backend ─────────
FROM golang:1.26-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download
COPY backend/ ./backend/
# Copy the compiled web UI into the embed dir before `go build` so
# go:embed picks it up.
COPY --from=web /src/web/out/ ./backend/internal/webui/dist/
RUN printf '*\n!.gitignore\n!.embedded\n' > ./backend/internal/webui/dist/.gitignore && \
    printf 'Docker build populated this dir.\n' > ./backend/internal/webui/dist/.embedded
ARG VERSION=dev
ARG COMMIT=unknown
RUN cd backend && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w \
    -X github.com/qorven/qorven/cmd.Version=${VERSION} \
    -X github.com/qorven/qorven/cmd.Commit=${COMMIT}" \
    -o /qorven .

# ───────── runtime ─────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /qorven /usr/local/bin/qorven
# 4200 = API, 443 = HTTPS web UI. Users can map whichever they need.
EXPOSE 4200 443
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s \
  CMD wget -qO- http://localhost:4200/livez || exit 1
ENTRYPOINT ["qorven"]
CMD ["start"]
