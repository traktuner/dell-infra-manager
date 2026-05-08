# ── Stage 1: Frontend ────────────────────────────────────────────────────────
FROM node:24-alpine AS frontend
WORKDIR /app/frontend

COPY frontend/package*.json ./
# npm ci is strict (requires lock file to match package.json exactly).
# npm install regenerates the lock file when there's a mismatch — needed when
# Renovate updates package.json but the lock file is stale (e.g. peer deps shift).
# We prefer ci for reproducibility; fall back to install only if ci fails.
RUN --mount=type=cache,target=/root/.npm \
    npm ci || npm install

COPY frontend/ ./
RUN npm run build

# ── Stage 2: Backend ─────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS backend
WORKDIR /app/backend

# Copy both manifests so this layer is only invalidated when dependencies change.
COPY backend/go.mod backend/go.sum ./
# --mount=type=cache keeps downloaded modules between builds.
RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

COPY backend/ ./
COPY --from=frontend /app/frontend/build ./frontend/dist

# Two caches:
#   /root/go/pkg/mod      — module source (shared with download step above)
#   /root/.cache/go-build — compiled package cache (unchanged pkgs are not recompiled)
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-w -s" -o dell-infra-manager .

# ── Stage 3: Final image ──────────────────────────────────────────────────────
FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend /app/backend/dell-infra-manager ./
RUN mkdir -p /data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q --spider http://localhost:8080/api/v1/dashboard || exit 1
ENTRYPOINT ["./dell-infra-manager"]
