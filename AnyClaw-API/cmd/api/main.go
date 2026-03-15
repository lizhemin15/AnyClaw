package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/adminconfig"
	"github.com/anyclaw/anyclaw-api/internal/adminstats"
	"github.com/anyclaw/anyclaw-api/internal/auth"
	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/energy"
	"github.com/anyclaw/anyclaw-api/internal/hosts"
	"github.com/anyclaw/anyclaw-api/internal/instances"
	"github.com/anyclaw/anyclaw-api/internal/llm"
	"github.com/anyclaw/anyclaw-api/internal/messages"
	"github.com/anyclaw/anyclaw-api/internal/usage"
	"github.com/anyclaw/anyclaw-api/internal/scheduler"
	"github.com/anyclaw/anyclaw-api/internal/setup"
	"github.com/anyclaw/anyclaw-api/internal/web"
	"github.com/anyclaw/anyclaw-api/internal/ws"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfgPath := flag.String("config", "", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}

	// Setup mode: no DB configured (config path for save)
	configPath := *cfgPath
	if configPath == "" {
		configPath = config.ConfigPath()
	}
	if cfg.DBDSN == "" {
		runSetupMode(configPath, cfg)
		return
	}

	database, err := db.Open(cfg.DBDSN)
	if err != nil {
		log.Printf("[db] connect failed: %v - running in setup mode", err)
		runSetupMode(configPath, cfg)
		return
	}
	defer database.Close()

	runApp(configPath, cfg, database)
}

func runSetupMode(cfgPath string, cfg *config.Config) {
	setupHandler := setup.New(cfgPath)
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.GetHead)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/api/setup/status", setupHandler.Status)
	r.Post("/api/setup/db", setupHandler.ConfigureDB)
	r.Post("/api/setup/admin", setupHandler.CreateAdmin)

	if h, err := web.SPAHandler(); err == nil {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })
	}

	addr := ":" + fmt.Sprintf("%d", cfg.Port)
	log.Printf("AnyClaw setup mode on %s - configure database at /setup", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func runApp(configPath string, cfg *config.Config, database *db.DB) {
	config.LoadFromDB = func() ([]byte, error) { return database.GetAdminConfigJSON() }
	if cfg2, err := config.Load(configPath); err == nil {
		*cfg = *cfg2
	}
	adminConfigHandler := adminconfig.New(configPath, database)
	authSvc := auth.New(database, cfg.JWTSecret, configPath)
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	sched := scheduler.New(apiURL, cfg.DockerImage, configPath, database)
	instHandler := instances.New(database, sched, apiURL, configPath)
	hostChecker := scheduler.HostChecker{}
	hostHandler := hosts.New(database, hostChecker, sched, apiURL, cfg.DockerImage)
	adminStatsHandler := adminstats.New(database)

	wsHub := ws.NewHub()
	wsHandler := ws.NewHandler(database, wsHub)
	msgHandler := messages.New(database)
	usageHandler := usage.New(database)
	energyHandler := energy.New(database, configPath, authSvc)
	proxy := llm.New(configPath, database, database)
	proxy.StartKeepAlive(5 * time.Minute)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.GetHead)
	if mw, err := web.SPAHTMLMiddleware(); err == nil {
		r.Use(mw)
	}

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/api/setup/status", func(w http.ResponseWriter, _ *http.Request) {
		var n int
		_ = database.QueryRow("SELECT COUNT(*) FROM users WHERE role='admin'").Scan(&n)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"configured": n > 0, "needs_admin_only": n == 0})
	})
	setupHandler := setup.New(configPath)
	r.Post("/api/setup/db", setupHandler.ConfigureDB)
	r.Post("/api/setup/admin", setupHandler.CreateAdmin)

	r.Route("/auth", func(r chi.Router) {
		r.Get("/config", authSvc.HandleAuthConfig)
		r.Post("/send-code", authSvc.HandleSendCode)
		r.Post("/register", authSvc.HandleRegister)
		r.Post("/login", authSvc.HandleLogin)
	})

	r.Route("/me", func(r chi.Router) {
		r.Use(authSvc.Middleware)
		r.Get("/", authSvc.HandleMe)
		r.Get("/usage", usageHandler.ListMyUsage)
	})

	r.Route("/instances", func(r chi.Router) {
		r.Use(authSvc.Middleware)
		r.Get("/", instHandler.List)
		r.Post("/", instHandler.Create)
		r.Get("/{id}/ws", wsHandler.HandleUserWS)
		r.Get("/{id}/messages", msgHandler.List)
		r.Put("/{id}/read", instHandler.MarkRead)
		r.Post("/{id}/subscribe", instHandler.Subscribe)
		r.Get("/{id}", instHandler.Get)
		r.Delete("/{id}", instHandler.Delete)
	})

	r.Get("/energy/config", energyHandler.GetPublicConfig)
	r.Route("/energy", func(r chi.Router) {
		r.Use(authSvc.Middleware)
		r.Get("/recharge/plans", energyHandler.GetRechargePlans)
	})
	r.Route("/admin", func(r chi.Router) {
		r.Use(authSvc.AdminMiddleware)
		r.Get("/config", adminConfigHandler.GetConfig)
		r.Get("/config/channel-status", func(w http.ResponseWriter, r *http.Request) {
			cfg, err := config.Load(configPath)
			if err != nil {
				http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
				return
			}
			channels := cfg.Channels
			if channels == nil {
				channels = []config.Channel{}
			}
			status := proxy.GetChannelStatus(channels)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"status": status})
		})
		r.Put("/config", adminConfigHandler.PutConfig)
		r.Post("/config/test", adminConfigHandler.TestChannel)
		r.Post("/config/test-smtp", adminConfigHandler.TestSMTP)
		r.Post("/db/check-and-migrate", func(w http.ResponseWriter, r *http.Request) {
			if err := database.CheckAndMigrate(); err != nil {
				log.Printf("[admin] db check-and-migrate failed: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "数据库结构已检查并修复"})
		})
		r.Post("/db/reset", func(w http.ResponseWriter, r *http.Request) {
			if err := database.Reset(); err != nil {
				log.Printf("[admin] db reset failed: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})
		r.Get("/stats", adminStatsHandler.GetStats)
		r.Get("/usage", usageHandler.ListAdminUsage)
		r.Post("/config/usage-correction", usageHandler.SetUsageCorrection)
		r.Get("/energy/users", energyHandler.ListUsers)
		r.Post("/energy/users", energyHandler.AdminCreateUser)
		r.Put("/energy/users/{id}", energyHandler.AdminUpdateUser)
		r.Post("/energy/recharge", energyHandler.Recharge)
		r.Post("/energy/daily", energyHandler.RunDaily)
		r.Post("/energy/users/{id}/recharge", energyHandler.AdminRechargeUser)
	})
	r.Route("/admin/hosts", func(r chi.Router) {
		r.Use(authSvc.AdminMiddleware)
		r.Get("/", hostHandler.List)
		r.Post("/", hostHandler.Create)
		r.Get("/{id}", hostHandler.Get)
		r.Put("/{id}", hostHandler.Update)
		r.Delete("/{id}", hostHandler.Delete)
		r.Post("/{id}/check", hostHandler.CheckStatus)
		r.Get("/{id}/instance-image-status", hostHandler.InstanceImageStatus)
		r.Get("/{id}/metrics", hostHandler.HostMetrics)
		r.Post("/{id}/pull-and-restart-instances", hostHandler.PullAndRestartInstances)
		r.Post("/{id}/prune-images", hostHandler.PruneImages)
		r.Post("/{id}/drain", hostHandler.Drain)
	})
	r.Route("/admin/instances", func(r chi.Router) {
		r.Use(authSvc.AdminMiddleware)
		r.Get("/", instHandler.AdminList)
		r.Post("/reconnect", instHandler.AdminReconnect)
		r.Post("/{id}/migrate", instHandler.AdminMigrate)
		r.Delete("/{id}", instHandler.AdminDelete)
	})

	r.Get("/containers/connect", wsHandler.HandleContainerConnect)

	r.HandleFunc("/llm/v1/chat/completions", proxy.HandleChatCompletions)

	if h, err := web.SPAHandler(); err == nil {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })
	}

	addr := ":" + fmt.Sprintf("%d", cfg.Port)
	log.Printf("AnyClaw-API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
