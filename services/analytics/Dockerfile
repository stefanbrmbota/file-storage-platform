# syntax=docker/dockerfile:1
# analytics servis. Build iz root-a repozitorijuma:
#   docker build -f services/analytics/Dockerfile -t fsp-analytics .

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY pkg/ ./pkg/

# go.sum* jer modul bez spoljnih zavisnosti nema go.sum.
COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/
WORKDIR /src/services/analytics
RUN --mount=type=cache,target=/go/pkg/mod     go mod download && go mod verify

COPY services/analytics/ ./

RUN --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"     -o /out/analytics ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/analytics /app/analytics

USER nonroot:nonroot

EXPOSE 8085

ENTRYPOINT ["/app/analytics"]
