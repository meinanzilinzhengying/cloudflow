module github.com/meinanzilinzhengying/cloudflow/services/shared/auth

go 1.22

require (
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	google.golang.org/grpc v1.80.0
)

replace github.com/meinanzilinzhengying/cloudflow/services/proto => ../../proto

replace github.com/meinanzilinzhengying/cloudflow/pkg => ../../../pkg
