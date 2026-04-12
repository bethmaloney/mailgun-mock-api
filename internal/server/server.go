package server

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/bethmaloney/mailgun-mock-api/internal/allowlist"
	"github.com/bethmaloney/mailgun-mock-api/internal/apikey"
	"github.com/bethmaloney/mailgun-mock-api/internal/auth"
	"github.com/bethmaloney/mailgun-mock-api/internal/config"
	"github.com/bethmaloney/mailgun-mock-api/internal/credential"
	"github.com/bethmaloney/mailgun-mock-api/internal/domain"
	"github.com/bethmaloney/mailgun-mock-api/internal/event"
	"github.com/bethmaloney/mailgun-mock-api/internal/ip"
	"github.com/bethmaloney/mailgun-mock-api/internal/mailinglist"
	"github.com/bethmaloney/mailgun-mock-api/internal/message"
	"github.com/bethmaloney/mailgun-mock-api/internal/metrics"
	appMiddleware "github.com/bethmaloney/mailgun-mock-api/internal/middleware"
	"github.com/bethmaloney/mailgun-mock-api/internal/mock"
	"github.com/bethmaloney/mailgun-mock-api/internal/route"
	"github.com/bethmaloney/mailgun-mock-api/internal/subaccount"
	"github.com/bethmaloney/mailgun-mock-api/internal/suppression"
	"github.com/bethmaloney/mailgun-mock-api/internal/tag"
	"github.com/bethmaloney/mailgun-mock-api/internal/template"
	"github.com/bethmaloney/mailgun-mock-api/internal/webhook"
	ws "github.com/bethmaloney/mailgun-mock-api/internal/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"
)

//go:embed all:static
var staticFiles embed.FS

func New(ctx context.Context, db *gorm.DB, cfg *config.Config, v *auth.Validator) http.Handler {
	validator := v

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Run model migrations.
	db.AutoMigrate(&domain.Domain{}, &domain.DNSRecord{}, &credential.SMTPCredential{}, &apikey.APIKey{}, &apikey.ManagedAPIKey{}, &allowlist.IPAllowlistEntry{}, &message.StoredMessage{}, &message.Attachment{}, &event.Event{},
		&suppression.Bounce{}, &suppression.Complaint{}, &suppression.Unsubscribe{}, &suppression.AllowlistEntry{},
		&template.Template{}, &template.TemplateVersion{},
		&tag.Tag{},
		&mailinglist.MailingList{}, &mailinglist.MailingListMember{},
		&webhook.DomainWebhook{}, &webhook.AccountWebhook{}, &webhook.WebhookDelivery{},
		&route.Route{},
		&ip.IP{}, &ip.IPPool{}, &ip.IPPoolIP{}, &ip.DomainIP{}, &ip.DomainPool{},
		&subaccount.Subaccount{}, &subaccount.SendingLimit{})

	// WebSocket hub
	hub := ws.NewHub()
	go hub.Run()

	// Mock management routes
	h := mock.NewHandlers(db, cfg)

	// Domain API routes
	dh := domain.NewHandlers(db, h.Config())

	// Credential API handlers
	ch := credential.NewHandlers(db)

	// API Key handlers
	kh := apikey.NewHandlers(db)

	// IP Allowlist handlers
	ah := allowlist.NewHandlers(db)

	// Event handlers
	eh := event.NewHandlers(db, h.Config())

	// Suppression handlers
	sh := suppression.NewHandlers(db)

	// Template handlers
	th := template.NewHandlers(db)

	// Tag handlers
	tgh := tag.NewHandlers(db)

	// Mailing list handlers
	mlh := mailinglist.NewHandlers(db)

	// Webhook handlers
	wh := webhook.NewHandlers(db, h.Config())

	// Route handlers
	rth := route.NewHandlers(db)

	// IP handlers
	iph := ip.NewHandlers(db)

	// Subaccount handlers
	sah := subaccount.NewHandlers(db)

	// Message handlers
	mh := message.NewHandlers(db, h.Config())
	mh.SetEventHandlers(eh)

	// Wire up WebSocket broadcasting
	h.SetHub(hub)
	mh.SetHub(hub)
	eh.SetHub(hub)

	// Metrics handlers
	mtrH := metrics.NewHandlers(db)

	// Account-level stats (v3) — register before wildcard {domain_name} routes
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/stats/total", mtrH.GetAccountStats)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/stats/filter", mtrH.GetFilteredStats)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/stats/total/domains", mtrH.GetDomainStatsSnapshot)

	r.Route("/v4/domains", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Use(appMiddleware.SubaccountScoping(db))
		r.Post("/", dh.CreateDomain)
		r.Get("/", dh.ListDomains)
		r.Get("/{name}", dh.GetDomain)
		r.Put("/{name}", dh.UpdateDomain)
		r.Put("/{name}/verify", dh.VerifyDomain)
		// v4 webhook routes
		r.Post("/{name}/webhooks", wh.V4CreateWebhook)
		r.Put("/{name}/webhooks", wh.V4UpdateWebhook)
		r.Delete("/{name}/webhooks", wh.V4DeleteWebhook)
	})

	r.Route("/v3/domains", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
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

		// Domain IP routes
		r.Get("/{name}/ips", iph.ListDomainIPs)
		r.Post("/{name}/ips", iph.AssignDomainIP)
		r.Delete("/{name}/ips/{ip}", iph.UnassignDomainIP)
	})

	// v3 domain webhook routes
	r.Route("/v3/domains/{domain_name}/webhooks", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", wh.ListWebhooks)
		r.Post("/", wh.CreateWebhook)
		r.Get("/{webhook_name}", wh.GetWebhook)
		r.Put("/{webhook_name}", wh.UpdateWebhook)
		r.Delete("/{webhook_name}", wh.DeleteWebhook)
	})

	// Subaccount routes
	r.Route("/v5/accounts/subaccounts", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Post("/", sah.CreateSubaccount)
		r.Get("/", sah.ListSubaccounts)
		r.Delete("/", sah.DeleteSubaccount)
		r.Get("/{subaccount_id}", sah.GetSubaccount)
		r.Post("/{subaccount_id}/disable", sah.DisableSubaccount)
		r.Post("/{subaccount_id}/enable", sah.EnableSubaccount)
		r.Get("/{subaccount_id}/limit/custom/monthly", sah.GetSendingLimit)
		r.Put("/{subaccount_id}/limit/custom/monthly", sah.SetSendingLimit)
		r.Delete("/{subaccount_id}/limit/custom/monthly", sah.RemoveSendingLimit)
		r.Put("/{subaccount_id}/features", sah.UpdateFeatures)
	})

	// v5 signing key routes
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v5/accounts/http_signing_key", wh.GetSigningKey)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Post("/v5/accounts/http_signing_key", wh.RegenerateSigningKey)

	// v1 account webhook routes
	r.Route("/v1/webhooks", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", wh.ListAccountWebhooks)
		r.Post("/", wh.CreateAccountWebhook)
		r.Delete("/", wh.BulkDeleteAccountWebhooks)
		r.Get("/{webhook_id}", wh.GetAccountWebhook)
		r.Put("/{webhook_id}", wh.UpdateAccountWebhook)
		r.Delete("/{webhook_id}", wh.DeleteAccountWebhook)
	})

	// Route CRUD (account-level, not domain-scoped)
	r.Route("/v3/routes", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Post("/", rth.CreateRoute)
		r.Get("/", rth.ListRoutes)
		r.Get("/match", rth.MatchRoute)
		r.Get("/{route_id}", rth.GetRoute)
		r.Put("/{route_id}", rth.UpdateRoute)
		r.Delete("/{route_id}", rth.DeleteRoute)
	})

	// Message sending route
	r.Route("/v3/{domain_name}/messages", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Post("/", mh.SendMessage)
	})

	// Events route
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/{domain_name}/events", eh.ListEvents)

	// MIME message sending route
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Post("/v3/{domain_name}/messages.mime", mh.SendMIMEMessage)

	// Delete envelopes (purge queue) route
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Delete("/v3/{domain_name}/envelopes", mh.DeleteEnvelopes)

	// Message storage routes (retrieve / delete / resend)
	r.Route("/v3/domains/{domain_name}/messages", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/{storage_key}", mh.GetMessage)
		r.Delete("/{storage_key}", mh.DeleteMessage)
		r.Post("/{storage_key}", mh.ResendMessage)
		r.Get("/{storage_key}/attachments/{attachment_id}", mh.GetAttachment)
	})

	// Sending queues route
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/domains/{domain_name}/sending_queues", mh.GetSendingQueues)

	// Suppression routes
	r.Route("/v3/{domain_name}/bounces", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", sh.ListBounces)
		r.Post("/", sh.CreateBounces)
		r.Post("/import", sh.ImportBounces)
		r.Get("/{address}", sh.GetBounce)
		r.Delete("/{address}", sh.DeleteBounce)
		r.Delete("/", sh.ClearBounces)
	})
	r.Route("/v3/{domain_name}/complaints", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", sh.ListComplaints)
		r.Post("/", sh.CreateComplaints)
		r.Post("/import", sh.ImportComplaints)
		r.Get("/{address}", sh.GetComplaint)
		r.Delete("/{address}", sh.DeleteComplaint)
		r.Delete("/", sh.ClearComplaints)
	})
	r.Route("/v3/{domain_name}/unsubscribes", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", sh.ListUnsubscribes)
		r.Post("/", sh.CreateUnsubscribes)
		r.Post("/import", sh.ImportUnsubscribes)
		r.Get("/{address}", sh.GetUnsubscribe)
		r.Delete("/{address}", sh.DeleteUnsubscribe)
		r.Delete("/", sh.ClearUnsubscribes)
	})
	r.Route("/v3/{domain_name}/whitelists", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", sh.ListAllowlist)
		r.Post("/", sh.CreateAllowlistEntry)
		r.Post("/import", sh.ImportAllowlist)
		r.Get("/{value}", sh.GetAllowlistEntry)
		r.Delete("/{value}", sh.DeleteAllowlistEntry)
		r.Delete("/", sh.ClearAllowlist)
	})

	// Template routes
	r.Route("/v3/{domain_name}/templates", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Post("/", th.CreateTemplate)
		r.Get("/", th.ListTemplates)
		r.Delete("/", th.DeleteAllTemplates)
		r.Get("/{name}", th.GetTemplate)
		r.Put("/{name}", th.UpdateTemplate)
		r.Delete("/{name}", th.DeleteTemplate)
		r.Post("/{name}/versions", th.CreateVersion)
		r.Get("/{name}/versions", th.ListVersions)
		r.Get("/{name}/versions/{tag}", th.GetVersion)
		r.Put("/{name}/versions/{tag}", th.UpdateVersion)
		r.Delete("/{name}/versions/{tag}", th.DeleteVersion)
		r.Put("/{name}/versions/{tag}/copy/{new_tag}", th.CopyVersion)
	})

	// Tag routes
	r.Route("/v3/{domain_name}/tags", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", tgh.ListTags)
		r.Get("/{tag}", tgh.GetTag)
		r.Put("/{tag}", tgh.UpdateTag)
		r.Delete("/{tag}", tgh.DeleteTag)

		// Tag stats routes
		r.Get("/{tag}/stats", tgh.GetTagStats)
		r.Get("/{tag}/stats/aggregates/countries", tgh.GetTagStatsCountries)
		r.Get("/{tag}/stats/aggregates/providers", tgh.GetTagStatsProviders)
		r.Get("/{tag}/stats/aggregates/devices", tgh.GetTagStatsDevices)
	})

	// Singular tag paths (OpenAPI spec style — tag from query parameter)
	r.Route("/v3/{domain_name}/tag", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", tgh.GetTagByQuery)
		r.Get("/stats", tgh.GetTagStatsByQuery)
		r.Get("/stats/aggregates/countries", tgh.GetTagStatsCountriesByQuery)
		r.Get("/stats/aggregates/providers", tgh.GetTagStatsProvidersByQuery)
		r.Get("/stats/aggregates/devices", tgh.GetTagStatsDevicesByQuery)
	})

	// Domain-level stats
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/{domain_name}/stats/total", tgh.GetDomainStats)

	// Domain aggregate stubs (v3)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/{domain_name}/aggregates/providers", mtrH.GetDomainAggregateProviders)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/{domain_name}/aggregates/devices", mtrH.GetDomainAggregateDevices)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/{domain_name}/aggregates/countries", mtrH.GetDomainAggregateCountries)

	// Tag limits route (different path pattern)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/domains/{domain_name}/limits/tag", tgh.GetTagLimits)

	// v1 Analytics Tags API (account-level, not domain-scoped)
	r.Route("/v1/analytics/tags", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Post("/", tgh.V1ListTags)
		r.Put("/", tgh.V1UpdateTag)
		r.Delete("/", tgh.V1DeleteTag)
		r.Get("/limits", tgh.V1GetTagLimits)
	})

	// v1 analytics metrics
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Post("/v1/analytics/metrics", mtrH.QueryMetrics)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Post("/v1/analytics/usage/metrics", mtrH.QueryUsageMetrics)

	// v2 bounce classification
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Post("/v2/bounce-classification/metrics", mtrH.QueryBounceClassification)

	// Mailing list routes
	r.Route("/v3/lists", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Post("/", mlh.CreateList)
		r.Get("/", mlh.ListListsLegacy)
		r.Get("/pages", mlh.ListLists)
		r.Get("/{list_address}", mlh.GetList)
		r.Put("/{list_address}", mlh.UpdateList)
		r.Delete("/{list_address}", mlh.DeleteList)
		r.Post("/{list_address}/members", mlh.AddMember)
		r.Get("/{list_address}/members", mlh.ListMembersLegacy)
		r.Get("/{list_address}/members/pages", mlh.ListMembers)
		r.Get("/{list_address}/members/{member_address}", mlh.GetMember)
		r.Put("/{list_address}/members/{member_address}", mlh.UpdateMember)
		r.Delete("/{list_address}/members/{member_address}", mlh.DeleteMember)
		r.Post("/{list_address}/members.json", mlh.BulkAddMembers)
		r.Post("/{list_address}/members.csv", mlh.CSVImportMembers)
	})

	// API Key routes
	r.Route("/v1/keys", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", kh.ListKeys)
		r.Post("/", kh.CreateKey)
		r.Get("/public", kh.GetPublicKey)
		r.Post("/public", kh.RegeneratePublicKey)
		r.Delete("/{id}", kh.DeleteKey)
		r.Post("/{id}/regenerate", kh.RegenerateKey)
	})

	// IP Allowlist routes
	r.Route("/v2/ip_whitelist", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", ah.ListEntries)
		r.Post("/", ah.AddEntry)
		r.Put("/", ah.UpdateEntry)
		r.Delete("/", ah.DeleteEntry)
	})

	// IP routes (account-level)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/ips", iph.ListIPs)
	r.With(appMiddleware.APIAuth(h.Config(), db, validator)).Get("/v3/ips/{ip}", iph.GetIP)

	// IP Pool routes (v1 prefix)
	r.Route("/v1/ip_pools", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", iph.ListPools)
		r.Post("/", iph.CreatePool)
		r.Get("/{pool_id}", iph.GetPool)
		r.Patch("/{pool_id}", iph.UpdatePool)
		r.Delete("/{pool_id}", iph.DeletePool)
	})

	// IP Pool routes (v3 prefix -- same handlers)
	r.Route("/v3/ip_pools", func(r chi.Router) {
		r.Use(appMiddleware.APIAuth(h.Config(), db, validator))
		r.Get("/", iph.ListPools)
		r.Post("/", iph.CreatePool)
		r.Get("/{pool_id}", iph.GetPool)
		r.Patch("/{pool_id}", iph.UpdatePool)
		r.Delete("/{pool_id}", iph.DeletePool)
	})

	// Managed API key handlers (init clears existing data — must be outside route group)
	mkh := apikey.NewManagedHandlers(db)

	// Mailgun API routes (placeholder)
	r.Route("/api/v3", func(r chi.Router) {
	})

	// Unauthenticated mock routes — registered before the /mock group so they
	// do not inherit the EntraRequired middleware.
	r.Get("/mock/health", mock.HealthHandler)
	r.Get("/mock/auth-config", h.GetAuthConfig)

	r.Route("/mock", func(r chi.Router) {
		r.Use(appMiddleware.EntraRequired(validator))

		// WebSocket route with log scrubber and optional reauth timeout
		if validator != nil {
			r.With(appMiddleware.WSLogScrubber()).Get("/ws", func(w http.ResponseWriter, req *http.Request) {
				hub.HandleWebSocketWithTimeout(w, req, 30*time.Minute)
			})
		} else {
			r.With(appMiddleware.WSLogScrubber()).Get("/ws", hub.HandleWebSocket)
		}

		// Managed API key routes
		r.Get("/api-keys", mkh.List)
		r.Post("/api-keys", mkh.Create)
		r.Delete("/api-keys/{id}", mkh.Delete)

		r.Get("/config", h.GetConfig)
		r.Put("/config", h.UpdateConfig)
		r.Get("/dashboard", h.GetDashboard)
		r.Post("/reset", h.ResetAll)
		// Order matters: "/reset/messages" must be registered before "/reset/{domain}"
		// so that chi matches the static path before the wildcard.
		r.Post("/reset/messages", h.ResetMessages)
		r.Post("/reset/{domain}", h.ResetDomain)
		// Mock messages (Web UI)
		r.Get("/messages", h.ListMessages)
		r.Post("/messages/clear", h.ClearAllMessages)
		r.Get("/messages/{message_id}", h.GetMessageDetail)
		r.Delete("/messages/{message_id}", h.DeleteSingleMessage)
		// Mock inbound simulation
		r.Post("/inbound/{domain}", rth.SimulateInbound)
		// Mock webhook inspection
		r.Get("/webhooks/deliveries", wh.ListDeliveries)
		r.Post("/webhooks/trigger", wh.TriggerWebhook)
		// Mock event triggers
		r.Route("/events/{domain}", func(r chi.Router) {
			r.Post("/deliver/{message_id}", eh.TriggerDeliver)
			r.Post("/fail/{message_id}", eh.TriggerFail)
			r.Post("/open/{message_id}", eh.TriggerOpen)
			r.Post("/click/{message_id}", eh.TriggerClick)
			r.Post("/unsubscribe/{message_id}", eh.TriggerUnsubscribe)
			r.Post("/complain/{message_id}", eh.TriggerComplain)
		})
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
