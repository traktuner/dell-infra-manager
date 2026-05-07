# Stage 1: Frontend Build
FROM node:24-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Backend Build (no CGO needed — pure Go SQLite)
FROM golang:1.26-alpine AS backend
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /app/frontend/build ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o dell-manager .

# Stage 3: Final minimal image
FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend /app/backend/dell-manager ./
RUN mkdir -p /data
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:8080/api/v1/dashboard || exit 1
ENTRYPOINT ["./dell-manager"]
