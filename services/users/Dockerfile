# syntax=docker/dockerfile:1
# Users servis. Build iz root-a repozitorijuma:
#   docker build -f services/users/Dockerfile -t fsp-users .

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY pkg/ ./pkg/

# Healthcheck program (pkg/healthcheck): distroless nema curl/shell, pa compose
# poziva /app/healthcheck <port> da proveri GET /health.
RUN --mount=type=cache,target=/root/.cache/go-build \
    cd /src/pkg/healthcheck && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck .

COPY services/users/go.mod services/users/go.sum ./services/users/
WORKDIR /src/services/users
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY services/users/ ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/users ./cmd/server

# distroless/static sadrži CA sertifikate (potrebni za SMTP/TLS ka spoljnim serverima).
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/users /app/users
COPY --from=build /out/healthcheck /app/healthcheck

USER nonroot:nonroot

EXPOSE 8081

ENTRYPOINT ["/app/users"]
