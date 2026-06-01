module cloud-flow/services/shared/auth

go 1.22

require (
	cloud-flow/services/proto v0.0.0
	google.golang.org/grpc v1.80.0
)

replace (
	cloud-flow/services/proto => ../../proto
)
