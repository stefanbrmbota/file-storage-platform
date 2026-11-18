# syntax=docker/dockerfile:1
# sharing servis. Build iz root-a repozitorijuma:
#   docker build -f services/sharing/Dockerfile -t fsp-sharing .

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY pkg/ ./pkg/

# Healthcheck program (pkg/healthcheck): distroless nema curl/shell, pa compose
# poziva /app/healthcheck <port> da proveri GET /health.
RUN --mount=type=cache,target=/root/.cache/go-build \
    cd /src/pkg/healthcheck && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck .

# go.sum* jer modul bez spoljnih zavisnosti nema go.sum.
COPY services/sharing/go.mod services/sharing/go.sum* ./services/sharing/
WORKDIR /src/services/sharing
RUN --mount=type=cache,target=/go/pkg/mod     go mod download && go mod verify

COPY services/sharing/ ./

RUN --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"     -o /out/sharing ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/sharing /app/sharing
COPY --from=build /out/healthcheck /app/healthcheck

USER nonroot:nonroot

EXPOSE 8084

ENTRYPOINT ["/app/sharing"]
