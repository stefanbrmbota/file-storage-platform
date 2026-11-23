module github.com/stefanbrmbota/file-storage-platform/api-gateway

go 1.27.3

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/stefanbrmbota/file-storage-platform/pkg/envconfig v0.0.0
)

replace github.com/stefanbrmbota/file-storage-platform/pkg/envconfig => ../pkg/envconfig
