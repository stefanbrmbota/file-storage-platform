package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	svcclient "github.com/stefanbrmbota/file-storage-platform/services/users/internal/client"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/config"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/handler"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/mail"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/password"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/repository"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/service"
	"github.com/stefanbrmbota/file-storage-platform/services/users/internal/token"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("neispravna konfiguracija", "err", err)
		os.Exit(1)
	}
	params, err := password.ParamsFromEnv()
	if err != nil {
		logger.Error("neispravni argon2 parametri", "err", err)
		os.Exit(1)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		logger.Error("mongo connect failed", "err", err)
		os.Exit(1)
	}
	startCtx, cancelStart := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStart()
	if err := client.Ping(startCtx, nil); err != nil {
		logger.Error("mongo nije dostupan", "err", err)
		os.Exit(1)
	}

	userRepo, err := repository.NewUserRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("user repository init failed", "err", err)
		os.Exit(1)
	}

	// US-24: inicijalni admin nalog iz env promenljivih (ako vec ne postoji).
	if cfg.AdminConfigured() {
		created, err := service.EnsureAdmin(startCtx, userRepo, cfg.AdminEmail, cfg.AdminPassword, cfg.DefaultQuotaBytes, params)
		if err != nil {
			logger.Error("kreiranje admin naloga nije uspelo", "err", err)
			os.Exit(1)
		}
		if created {
			logger.Info("admin nalog kreiran", "email", cfg.AdminEmail)
		} else {
			logger.Info("admin nalog vec postoji", "email", cfg.AdminEmail)
		}
	} else {
		logger.Warn("ADMIN_EMAIL i ADMIN_PASSWORD nisu postavljeni, admin nalog se ne kreira")
	}

	tokenRepo, err := repository.NewTokenRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("token repository init failed", "err", err)
		os.Exit(1)
	}

	mailer := &mail.SMTPMailer{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		From:     cfg.SMTPFrom,
		User:     cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		Timeout:  cfg.RequestTimeout,
	}
	filesClient := svcclient.NewFilesClient(cfg.FilesServiceURL, cfg.RequestTimeout)
	confirmation := service.NewConfirmationService(userRepo, tokenRepo, mailer, filesClient, cfg.ConfirmationTTL, cfg.ConfirmURLBase)
	registration := service.NewRegistrationService(userRepo, confirmation, params, cfg.DefaultQuotaBytes)

	otpRepo, err := repository.NewOTPRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("otp repository init failed", "err", err)
		os.Exit(1)
	}
	otpService, err := service.NewOTPService(otpRepo, cfg.OTPHMACSecret, cfg.OTPTTL, cfg.OTPMaxAttempts)
	if err != nil {
		logger.Error("neispravna OTP konfiguracija (OTP_HMAC_SECRET, OTP_TTL)", "err", err)
		os.Exit(1)
	}
	tokens, err := token.NewManager(cfg.JWTSecret, cfg.JWTTTL)
	if err != nil {
		logger.Error("neispravna JWT konfiguracija (JWT_SECRET, JWT_TTL)", "err", err)
		os.Exit(1)
	}
	login, err := service.NewLoginService(userRepo, otpService, tokens, mailer, cfg.OTPTTL, params, cfg.PasswordMaxAge)
	if err != nil {
		logger.Error("login servis init failed (PASSWORD_MAX_AGE)", "err", err)
		os.Exit(1)
	}

	revokedRepo, err := repository.NewRevokedTokenRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("revoked token repository init failed", "err", err)
		os.Exit(1)
	}
	logout := service.NewLogoutService(tokens, revokedRepo)
	quota := service.NewQuotaService(userRepo)
	accounts, err := service.NewAccountService(userRepo, cfg.PasswordMaxAge)
	if err != nil {
		logger.Error("neispravna konfiguracija podataka o nalogu (PASSWORD_MAX_AGE)", "err", err)
		os.Exit(1)
	}
	passwords, err := service.NewPasswordService(userRepo, tokens, revokedRepo, params, cfg.PasswordMinAge)
	if err != nil {
		logger.Error("neispravna konfiguracija promene lozinke (ARGON2_*, PASSWORD_MIN_AGE)", "err", err)
		os.Exit(1)
	}

	resetRepo, err := repository.NewPasswordResetRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("password reset repository init failed", "err", err)
		os.Exit(1)
	}
	passwordReset, err := service.NewPasswordResetService(userRepo, resetRepo, otpRepo, mailer, service.PasswordResetConfig{
		BaseURL:  cfg.ResetURLBase,
		TTL:      cfg.PasswordResetTTL,
		Cooldown: cfg.PasswordResetCooldown,
		Params:   params,
	})
	if err != nil {
		logger.Error("neispravna konfiguracija reseta lozinke (RESET_URL_BASE, PASSWORD_RESET_*)", "err", err)
		os.Exit(1)
	}

	// US-17: povracaj naloga magicnim linkom.
	magicRepo, err := repository.NewMagicLinkRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("magic link repository init failed", "err", err)
		os.Exit(1)
	}
	magicLink, err := service.NewMagicLinkService(userRepo, magicRepo, mailer, service.MagicLinkConfig{
		BaseURL:  cfg.MagicLinkURLBase,
		TTL:      cfg.MagicLinkTTL,
		Cooldown: cfg.MagicLinkCooldown,
	})
	if err != nil {
		logger.Error("neispravna konfiguracija magicnog linka (MAGIC_LINK_URL_BASE, MAGIC_LINK_*)", "err", err)
		os.Exit(1)
	}
	magicLogin := service.NewMagicLoginService(userRepo, magicRepo, tokens)

	// US-21: zahtevi korisnika za povecanje kvote.
	quotaRequestRepo, err := repository.NewQuotaRequestRepository(startCtx, client.Database(cfg.MongoDB))
	if err != nil {
		logger.Error("quota request repository init failed", "err", err)
		os.Exit(1)
	}
	quotaRequests := service.NewQuotaRequestService(userRepo, quotaRequestRepo)
	// US-22: administratorski pregled korisnika i zahteva za kvotu.
	admin := service.NewAdminService(userRepo, userRepo, quotaRequestRepo)

	// US-20: interne rute za kvotu smeju da zovu samo servisi koji znaju INTERNAL_TOKEN.
	internalOnly, err := handler.RequireInternalToken(cfg.InternalToken)
	if err != nil {
		logger.Error("neispravna konfiguracija (INTERNAL_TOKEN)", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /register", handler.RegisterHandler(registration))
	mux.HandleFunc("POST /login", handler.LoginHandler(login))
	mux.HandleFunc("POST /login/verify", handler.LoginVerifyHandler(login))
	mux.HandleFunc("POST /confirm", handler.ConfirmHandler(confirmation))
	mux.HandleFunc("POST /logout", handler.LogoutHandler(logout))
	// US-06: nalog i kvota prijavljenog korisnika; identitet dolazi iz X-User-ID (postavlja gateway).
	mux.HandleFunc("GET /users/me", handler.MeHandler(accounts))
	// US-21: zahtev za povecanje kvote i status poslednjeg zahteva; identitet iz X-User-ID.
	mux.HandleFunc("POST /users/me/quota-requests", handler.SubmitQuotaRequestHandler(quotaRequests))
	mux.HandleFunc("GET /users/me/quota-requests/latest", handler.LatestQuotaRequestHandler(quotaRequests))
	// US-22: admin pregled; gateway propusta samo ulogu ADMIN, a servis ulogu proverava i u bazi.
	mux.HandleFunc("GET /admin/users", handler.AdminUsersHandler(admin))
	mux.HandleFunc("GET /admin/quota-requests", handler.AdminQuotaRequestsHandler(admin))
	// US-14: promena lozinke prijavljenog korisnika; posle uspeha se opoziva token zahteva.
	mux.HandleFunc("POST /users/me/password", handler.ChangePasswordHandler(passwords))
	// US-15: reset zaboravljene lozinke mejlom; javne rute (gateway ih ne trazi JWT).
	mux.HandleFunc("POST /password-reset/request", handler.PasswordResetRequestHandler(passwordReset))
	mux.HandleFunc("POST /password-reset/confirm", handler.PasswordResetConfirmHandler(passwordReset))
	// US-17: povracaj naloga magicnim linkom; javne rute (korisnik nije prijavljen).
	mux.HandleFunc("POST /magic-link/request", handler.MagicLinkRequestHandler(magicLink))
	mux.HandleFunc("POST /magic-link/login", handler.MagicLinkLoginHandler(magicLogin))
	mux.HandleFunc("POST /internal/tokens/check", handler.TokenCheckHandler(logout))
	mux.Handle("POST /internal/users/{id}/quota/reserve", internalOnly(handler.ReserveQuotaHandler(quota)))
	mux.Handle("POST /internal/users/{id}/quota/release", internalOnly(handler.ReleaseQuotaHandler(quota)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("users service started", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "err", err)
	}
	if err := client.Disconnect(ctx); err != nil {
		logger.Error("mongo disconnect failed", "err", err)
	}
	logger.Info("users service stopped")
}
