# syntax=docker/dockerfile:1
# gateway servis. Build iz root-a repozitorijuma:
#   docker build -f api-gateway/Dockerfile -t fsp-gateway .

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY pkg/ ./pkg/

# go.sum* jer modul bez spoljnih zavisnosti nema go.sum.
COPY api-gateway/go.mod api-gateway/go.sum* ./api-gateway/
WORKDIR /src/api-gateway
RUN --mount=type=cache,target=/go/pkg/mod     go mod download && go mod verify

COPY api-gateway/ ./

RUN --mount=type=cache,target=/go/pkg/mod     --mount=type=cache,target=/root/.cache/go-build     CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w"     -o /out/gateway ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/gateway /app/gateway

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/gateway"]
