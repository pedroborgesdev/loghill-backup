FROM node:22-alpine AS frontend-builder
WORKDIR /src/apps/frontend
COPY apps/frontend/package*.json ./
RUN npm ci
COPY apps/frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend-builder
WORKDIR /src/apps/backend
COPY apps/backend/go.mod apps/backend/go.sum* ./
RUN go mod download
COPY apps/backend/ ./
COPY --from=frontend-builder /src/apps/backend/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/logmate ./cmd/server

FROM alpine:3.22 AS runtime
RUN apk add --no-cache tzdata && \
    addgroup -S app && adduser -S -G app app && mkdir -p /app/data && chown -R app:app /app
WORKDIR /app
COPY --from=backend-builder /out/logmate /app/logmate
COPY docs/openapi.yaml /app/docs/openapi.yaml
USER app
ENV APP_HOST=0.0.0.0 APP_PORT=8080 DATA_DIR=/app/data
EXPOSE 8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/logmate"]
