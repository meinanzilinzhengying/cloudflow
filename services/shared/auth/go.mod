module cloudflow/services/shared/auth

go 1.22

require (
	cloudflow/services/proto v0.0.0
	google.golang.org/grpc v1.80.0
)

replace (
	cloudflow/services/proto => ../../proto
)
