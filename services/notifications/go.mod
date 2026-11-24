module github.com/stefanbrmbota/file-storage-platform/services/notifications

go 1.27.3

require (
	github.com/apache/cassandra-gocql-driver/v2 v2.1.2
	github.com/stefanbrmbota/file-storage-platform/pkg/envconfig v0.0.0
)

require gopkg.in/inf.v0 v0.9.1 // indirect

replace github.com/stefanbrmbota/file-storage-platform/pkg/envconfig => ../../pkg/envconfig
