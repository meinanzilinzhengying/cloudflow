module github.com/meinanzilinzhengying/cloudflow/services/alert-engine

go 1.22

require (
	github.com/meinanzilinzhengying/cloudflow/pkg v0.0.0
	github.com/prometheus/client_golang v1.21.1
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	google.golang.org/grpc v1.62.1
)

replace github.com/prometheus/common => github.com/prometheus/common v0.48.0
