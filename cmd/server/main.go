package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	connect "connectrpc.com/connect"
	"github.com/newrelic/go-agent/v3/integrations/nrconnect"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/synthify/backend/apps/api/internal/handler"
	objectstorage "github.com/synthify/backend/apps/api/internal/infrastructure/storage"
	stripebilling "github.com/synthify/backend/apps/api/internal/infrastructure/stripe"
	"github.com/synthify/backend/apps/api/internal/observability"
	"github.com/synthify/backend/apps/api/internal/service"
	"github.com/synthify/backend/apps/worker/pkg/worker"
	"github.com/synthify/backend/packages/shared/app"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/config"
	"github.com/synthify/backend/packages/shared/domain"
	treev1connect "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1/treev1connect"
	"github.com/synthify/backend/packages/shared/job/log"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/postgres"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadAPI()

	slogLogger := applog.NewJSONSlogLogger(os.Stdout)
	appLogger := applog.WrapSlogLogger(slogLogger)

	appCtx := app.Bootstrap(ctx, cfg.GCSUploadURLBase, cfg.FirebaseProjectID, appLogger)
	store := appCtx.Store
	notifier := appCtx.Notifier

	jobLogger := postgres.NewDBLogger(store)
	nrApp, err := observability.InitNewRelic(cfg.NewRelic, slogLogger)
	if err != nil {
		log.Fatalf("failed to initialize new relic: %v", err)
	}
	dispatcher := initDispatcher(cfg)
	billingProvider, err := initBillingProvider(cfg)
	if err != nil {
		log.Fatalf("failed to initialize billing provider: %v", err)
	}

	workspaceService := service.NewWorkspaceService(store, store, appLogger)
	billingService := service.NewBillingService(store, store, billingProvider, appLogger)
	documentService := service.NewDocumentService(store, store, app.NewDocumentSourceURLBuilder(cfg.InternalGCSUploadBase), objectstorage.NewObjectMetadataFetcher(cfg.InternalGCSUploadBase), dispatcher, notifier, appLogger)
	itemService := service.NewItemService(store, store, appLogger)
	treeHandler := handler.NewTreeHandler(store, store, store)
	jobHandler := handler.NewJobHandler(store, store, store, appLogger)

	connectHandlerOpts := newRelicConnectHandlerOptions(nrApp)

	mux := http.NewServeMux()
	mux.HandleFunc("/stripe/webhook", handler.NewBillingWebhookHTTPHandler(billingService, appLogger))
	mux.Handle(treev1connect.NewBillingServiceHandler(handler.NewBillingHandler(billingService), connectHandlerOpts...))
	mux.Handle(treev1connect.NewWorkspaceServiceHandler(handler.NewWorkspaceHandler(workspaceService, billingService, store), connectHandlerOpts...))
	mux.Handle(treev1connect.NewDocumentServiceHandler(handler.NewDocumentHandler(documentService, store, store, app.NewDocumentUploadURLBuilder(cfg.GCSUploadURLBase)), connectHandlerOpts...))
	mux.Handle(treev1connect.NewJobServiceHandler(jobHandler, connectHandlerOpts...))
	mux.Handle(treev1connect.NewTreeServiceHandler(treeHandler, connectHandlerOpts...))
	mux.Handle(treev1connect.NewItemServiceHandler(handler.NewItemHandler(itemService, store, store), connectHandlerOpts...))
	mux.HandleFunc(newrelic.WrapHandleFunc(nrApp, "/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	}))

	h := middleware.Recover(appLogger,
		middleware.Logger(appLogger,
			middleware.CORS(cfg.CORSAllowedOrigins,
				// log-viewer など read-only ツールは API を経由せず Postgres を直接参照する設計
				middleware.WithAuth(cfg.FirebaseProjectID, appLogger,
					withJobLogger(jobLogger, mux),
				),
			),
		),
	)

	addr := fmt.Sprintf(":%s", cfg.Port)
	appLogger.Info(ctx, "api.started", map[string]any{"addr": addr})
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

func initDispatcher(cfg config.API) service.WorkerDispatcher {
	if cfg.WorkerBaseURL != "" {
		return worker.NewHTTPDispatcher(cfg.WorkerBaseURL)
	}
	return nil
}

func initBillingProvider(cfg config.API) (service.BillingProvider, error) {
	provider, err := stripebilling.NewProvider(stripebilling.Config{
		SecretKey:        cfg.Stripe.SecretKey,
		WebhookSecret:    cfg.Stripe.WebhookSecret,
		ProPriceID:       cfg.Stripe.ProPriceID,
		ProPriceIDJPY:    cfg.Stripe.ProPriceIDJPY,
		ProPriceIDUSD:    cfg.Stripe.ProPriceIDUSD,
		DefaultCurrency:  cfg.Stripe.DefaultCurrency,
		SuccessURL:       cfg.Billing.SuccessURL,
		CancelURL:        cfg.Billing.CancelURL,
		PortalReturnURL:  cfg.Billing.PortalReturnURL,
		MeterInputEvent:  cfg.Stripe.MeterInputEvent,
		MeterOutputEvent: cfg.Stripe.MeterOutputEvent,
	})
	if errors.Is(err, domain.ErrBillingProviderNotConfigured) {
		return nil, nil
	}
	return provider, err
}

func withJobLogger(l joblog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := joblog.WithLogger(r.Context(), l)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRelicConnectHandlerOptions(app *newrelic.Application) []connect.HandlerOption {
	if app == nil {
		return nil
	}
	return []connect.HandlerOption{
		connect.WithInterceptors(nrconnect.Interceptor(app)),
	}
}
