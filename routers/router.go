package routers

import (
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/maltegrosse/go-minio-list/controllers"
	. "github.com/maltegrosse/go-minio-list/log"
	l "github.com/treastech/logger"
)

func Routes() *chi.Mux {
	router := chi.NewRouter()
	cors_ := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})

	router.Use(
		l.Logger(Log),
		middleware.RealIP,
		middleware.Recoverer,
		cors_.Handler,
	)
	// just use all patterns
	router.Get("/*", controllers.List)
	return router
}
