# syntax=docker/dockerfile:1.7

# 1. Frontend: static SvelteKit bundle
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 2. Backend: static Go binary with the bundle embedded
FROM golang:1.27-alpine AS server
WORKDIR /src
COPY server/go.mod server/go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY server/ ./
RUN rm -rf ui/build && mkdir -p ui/build
COPY --from=web /web/build ./ui/build
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /ttb .

# 3. Runtime: nothing but the binary
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=server /ttb /ttb
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/ttb"]
CMD ["-config", "/config/config.yaml"]
