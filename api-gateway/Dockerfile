# syntax=docker/dockerfile:1
# gateway servis. Build iz root-a repozitorijuma:
#   docker build -f api-gateway/Dockerfile -t fsp-gateway .

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY pkg/ ./pkg/

# Healthcheck program (pkg/healthcheck): distroless nema curl/shell, pa compose
# poziva /app/healthcheck <port> da proveri GET /health.
RUN --mount=type=cache,target=/root/.cache/go-build \
    cd /src/pkg/healthcheck && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck .

# go.sum* jer modul bez spoljnih zavisnosti nema go.sum.
COPY api-gateway/go.mod api-gateway/go.sum* ./api-gateway/
WORKDIR /src/api-gateway
RUN --mount=type=cache,target=/go/pkg/mod     go mod download && go mod verify

COPY api-gateway/ ./

RUN --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"     -o /out/gateway ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/gateway /app/gateway
COPY --from=build /out/healthcheck /app/healthcheck

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/gateway"]
