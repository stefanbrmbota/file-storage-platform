# syntax=docker/dockerfile:1
# Files servis. Build iz root-a repozitorijuma:
#   docker build -f services/files/Dockerfile -t fsp-files .

FROM golang:1.27-alpine AS build

WORKDIR /src

COPY pkg/ ./pkg/

COPY services/files/go.mod services/files/go.sum ./services/files/
WORKDIR /src/services/files
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY services/files/ ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/files ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/files /app/files

USER nonroot:nonroot

EXPOSE 8082

ENTRYPOINT ["/app/files"]
