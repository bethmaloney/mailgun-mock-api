package server

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/bethmaloney/mailgun-mock-api/internal/apikey"
	"github.com/bethmaloney/mailgun-mock-api/internal/credential"
	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	appMiddleware "github.com/bethmaloney/mailgun-mock-api/internal/middleware"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"gorm.io/gorm"
)

//go:embed all:static
var staticFiles embed.FS

func New(db *gorm.DB) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Run domain model migrations.
	db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}, &credential.SMTPCredential{}, &apikey.APIKey{})

	// Mock management routes
	h := mock.NewHandlers(db)

	// Domain API routes
	dh := domain.NewHandlers(db, h.Config())

	// Credential API handlers
	ch := credential.NewHandlers(db)

	// API Key handlers
	kh := apikey.NewHandlers(db)

	r.Route("/v4/domains", func(r chi.Router) {
		r.Use(appMiddleware.BasicAuth(h.Config()))
		r.Post("/", dh.CreateDomain)
		r.Get("/", dh.ListDomains)
		r.Get("/{name}", dh.GetDomain)
		r.Put("/{name}", dh.UpdateDomain)
		r.Put("/{name}/verify", dh.VerifyDomain)
	})

	r.Route("/v3/domains", func(r chi.Router) {
		r.Use(appMiddleware.BasicAuth(h.Config()))
		r.Delete("/{name}", dh.DeleteDomain)
		r.Get("/{name}/tracking", dh.GetTracking)
		r.Put("/{name}/tracking/open", dh.UpdateOpenTracking)
		r.Put("/{name}/tracking/click", dh.UpdateClickTracking)
		r.Put("/{name}/tracking/unsubscribe", dh.UpdateUnsubscribeTracking)
		r.Get("/{name}/connection", dh.GetConnection)
		r.Put("/{name}/connection", dh.UpdateConnection)
		r.Put("/{name}/dkim_authority", dh.UpdateDKIMAuthority)
		r.Put("/{name}/dkim_selector", dh.UpdateDKIMSelector)

		// Credential routes
		r.Get("/{name}/credentials", ch.ListCredentials)
		r.Post("/{name}/credentials", ch.CreateCredential)
		r.Delete("/{name}/credentials", ch.DeleteAllCredentials)
		r.Put("/{name}/credentials/{spec}", ch.UpdateCredential)
		r.Delete("/{name}/credentials/{spec}", ch.DeleteCredential)
	})

	// API Key routes
	r.Route("/v1/keys", func(r chi.Router) {
		r.Use(appMiddleware.BasicAuth(h.Config()))
		r.Get("/", kh.ListKeys)
		r.Post("/", kh.CreateKey)
		r.Get("/public", kh.GetPublicKey)
		r.Delete("/{id}", kh.DeleteKey)
		r.Post("/{id}/regenerate", kh.RegenerateKey)
	})

	// Mailgun API routes (placeholder)
	r.Route("/api/v3", func(r chi.Router) {
	})
	r.Route("/mock", func(r chi.Router) {
		r.Get("/health", mock.HealthHandler)
		r.Get("/config", h.GetConfig)
		r.Put("/config", h.UpdateConfig)
		r.Post("/reset", h.ResetAll)
		// Order matters: "/reset/messages" must be registered before "/reset/{domain}"
		// so that chi matches the static path before the wildcard.
		r.Post("/reset/messages", h.ResetMessages)
		r.Post("/reset/{domain}", h.ResetDomain)
	})

	// Serve embedded Vue SPA
	r.Handle("/*", spaHandler())

	return r
}

func spaHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		}

		// Check if file exists in embedded FS
		f, err := sub.Open(path[1:]) // strip leading /
		if err != nil {
			// File not found — serve index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
