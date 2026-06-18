module github.com/meinanzilinzhengying/cloudflow/services/tenant-service

go 1.22

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/prometheus/client_golang v1.23.2
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/shared v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/shared/auth v0.0.0
	google.golang.org/grpc v1.62.1
)

replace golang.org/x/sys => golang.org/x/sys v0.18.0
