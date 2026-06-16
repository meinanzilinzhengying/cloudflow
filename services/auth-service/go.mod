module github.com/meinanzilinzhengying/cloudflow/services/auth-service

go 1.22

require (
	github.com/meinanzilinzhengying/cloudflow/pkg v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/shared/auth v0.0.0
	github.com/prometheus/client_golang v1.21.1
	google.golang.org/grpc v1.62.1
)

replace github.com/prometheus/common => github.com/prometheus/common v0.48.0
replace golang.org/x/sys => golang.org/x/sys v0.18.0
