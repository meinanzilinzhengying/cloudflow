module github.com/meinanzilinzhengying/cloudflow/services/auth-service

go 1.25.0

require (
	github.com/casbin/casbin/v2 v2.135.0
	github.com/coreos/go-oidc/v3 v3.19.0
	github.com/go-redis/redis/v8 v8.11.5
	github.com/go-sql-driver/mysql v1.10.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/meinanzilinzhengying/cloudflow/pkg v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/proto v0.0.0
	github.com/meinanzilinzhengying/cloudflow/services/shared v0.0.0-00010101000000-000000000000
	github.com/meinanzilinzhengying/cloudflow/services/shared/resilience v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.52.0
	golang.org/x/oauth2 v0.36.0
	google.golang.org/grpc v1.71.0
	gorm.io/driver/mysql v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/casbin/govaluate v1.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/meinanzilinzhengying/cloudflow/services/shared/auth v0.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/redis/go-redis/v9 v9.7.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260120221211-b8f7ae30c516 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/meinanzilinzhengying/cloudflow/pkg => ../../pkg

replace github.com/meinanzilinzhengying/cloudflow/proto => ../../proto

replace github.com/meinanzilinzhengying/cloudflow/services/proto => ../../services/proto

replace github.com/meinanzilinzhengying/cloudflow/services/shared => ../../services/shared

replace github.com/meinanzilinzhengying/cloudflow/services/shared/auth => ../../services/shared/auth

replace github.com/meinanzilinzhengying/cloudflow/services/shared/resilience => ../../services/shared/resilience
