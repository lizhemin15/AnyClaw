package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

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
	log.Printf("OpenClaw setup mode on %s - configure database at /setup", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func runApp(configPath string, cfg *config.Config, database *db.DB) {
	authSvc := auth.New(database, cfg.JWTSecret)
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	sched := scheduler.New(apiURL, cfg.DockerImage, configPath, database)
	instHandler := instances.New(database, sched, apiURL)
	hostChecker := scheduler.HostChecker{}
	hostHandler := hosts.New(database, hostChecker)
	adminConfigHandler := adminconfig.New(configPath)
	adminStatsHandler := adminstats.New(database)

	wsHub := ws.NewHub()
	wsHandler := ws.NewHandler(database, wsHub)
	msgHandler := messages.New(database)
	energyHandler := energy.New(database)

	proxy := llm.New(configPath, database, database)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/api/setup/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"configured":true}`))
	})

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", authSvc.HandleRegister)
		r.Post("/login", authSvc.HandleLogin)
	})

	r.Route("/me", func(r chi.Router) {
		r.Use(authSvc.Middleware)
		r.Get("/", authSvc.HandleMe)
	})

	r.Route("/instances", func(r chi.Router) {
		r.Use(authSvc.Middleware)
		r.Get("/", instHandler.List)
		r.Post("/", instHandler.Create)
		r.Get("/{id}/ws", wsHandler.HandleUserWS)
		r.Get("/{id}/messages", msgHandler.List)
		r.Get("/{id}", instHandler.Get)
		r.Delete("/{id}", instHandler.Delete)
	})

	r.Route("/energy", func(r chi.Router) {
		r.Use(authSvc.Middleware)
		r.Post("/invite", energyHandler.InviteCode)
		r.Post("/invite/use", energyHandler.UseInviteCode)
	})
	r.Route("/admin", func(r chi.Router) {
		r.Use(authSvc.AdminMiddleware)
		r.Get("/config", adminConfigHandler.GetConfig)
		r.Put("/config", adminConfigHandler.PutConfig)
		r.Post("/config/test", adminConfigHandler.TestChannel)
		r.Get("/stats", adminStatsHandler.GetStats)
		r.Get("/energy/users", energyHandler.ListUsers)
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
	})

	r.Get("/containers/connect", wsHandler.HandleContainerConnect)

	r.HandleFunc("/llm/v1/chat/completions", proxy.HandleChatCompletions)

	if h, err := web.SPAHandler(); err == nil {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) })
	}

	addr := ":" + fmt.Sprintf("%d", cfg.Port)
	log.Printf("OpenClaw-API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
