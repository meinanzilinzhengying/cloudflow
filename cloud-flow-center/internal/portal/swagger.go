// Package portal 提供 HTTP 服务和 API 端点
package portal

import (
	"net/http"

	"github.com/swaggo/http-swagger"
)

// @title Cloud Flow API
// @version 1.0
// @description Cloud Flow 云内流量监测平台 API 文档
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.example.com/support
// @contact.email support@example.com

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api
// @schemes http https

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

// RegisterSwaggerRoutes 注册 Swagger 文档路由
func RegisterSwaggerRoutes(mux *http.ServeMux, authMiddleware func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("/swagger/*", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		httpSwagger.Handler(
			httpSwagger.URL("/swagger/doc.json"),
			httpSwagger.DeepLinking(true),
			httpSwagger.DocExpansion("none"),
			httpSwagger.DomID("swagger-ui"),
		).ServeHTTP(w, r)
	}))

	mux.Handle("/swagger/doc.json", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/swagger", http.FileServer(http.Dir("./swagger"))).ServeHTTP(w, r)
	})))
}
