module github.com/meinanzilinzhengying/cloudflow/services/tenant-service

go 1.24.0

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0-20260609072044-59a8402c9599
	github.com/meinanzilinzhengying/cloudflow/services/shared v0.0.0-20260609072044-59a8402c9599
	github.com/meinanzilinzhengying/cloudflow/services/shared/auth v0.0.0-20260609072044-59a8402c9599
	google.golang.org/grpc v1.80.0
)

replace github.com/meinanzilinzhengying/cloudflow/services/proto => ../proto

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
