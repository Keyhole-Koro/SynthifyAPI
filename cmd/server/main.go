package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	connect "connectrpc.com/connect"
	"github.com/newrelic/go-agent/v3/integrations/nrconnect"
	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/synthify/backend/apps/api/internal/handler"
	"github.com/synthify/backend/apps/api/internal/observability"
	"github.com/synthify/backend/apps/api/internal/service"
	"github.com/synthify/backend/apps/worker/pkg/worker"
	"github.com/synthify/backend/packages/shared/app"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/config"
	treev1connect "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1/treev1connect"
	"github.com/synthify/backend/packages/shared/joblog"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/postgres"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadAPI()

	appCtx := app.Bootstrap(ctx, cfg.GCSUploadURLBase, cfg.FirebaseProjectID)
	store := appCtx.Store
	notifier := appCtx.Notifier

	jobLogger := postgres.NewDBLogger(store)
	slogLogger := applog.NewJSONSlogLogger(os.Stdout)
	appLogger := applog.WrapSlogLogger(slogLogger)
	nrApp, err := observability.InitNewRelic(cfg, slogLogger)
	if err != nil {
		log.Fatalf("failed to initialize new relic: %v", err)
	}
	dispatcher := initDispatcher(cfg)

	workspaceService := service.NewWorkspaceService(store, store)
	billingService := service.NewBillingService(store, nil, appLogger)
	documentService := service.NewDocumentService(store, store, app.NewDocumentSourceURLBuilder(cfg.InternalGCSUploadBase), dispatcher, notifier)
	itemService := service.NewItemService(store, store)

	treeHandler := handler.NewTreeHandler(store, store, store)
	jobHandler := handler.NewJobHandler(store, store, store)
	connectHandlerOpts := newRelicConnectHandlerOptions(nrApp)

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewBillingServiceHandler(handler.NewBillingHandler(billingService), connectHandlerOpts...))
	mux.Handle(treev1connect.NewWorkspaceServiceHandler(handler.NewWorkspaceHandler(workspaceService, store), connectHandlerOpts...))
	mux.Handle(treev1connect.NewDocumentServiceHandler(handler.NewDocumentHandler(documentService, store, store, app.NewDocumentUploadURLBuilder(cfg.GCSUploadURLBase)), connectHandlerOpts...))
	mux.Handle(treev1connect.NewJobServiceHandler(jobHandler, connectHandlerOpts...))
	mux.Handle(treev1connect.NewTreeServiceHandler(treeHandler, connectHandlerOpts...))
	mux.Handle(treev1connect.NewItemServiceHandler(handler.NewItemHandler(itemService, store, store), connectHandlerOpts...))
	mux.HandleFunc(newrelic.WrapHandleFunc(nrApp, "/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	}))

	h := middleware.Recover(
		middleware.Logger(
			middleware.CORS(cfg.CORSAllowedOrigins,
				// Only allow anonymous read (for tools like log-viewer) in local development.
				// TODO: Move to service-level auth or restricted VPN access for tools in production.
				middleware.WithAuth(cfg.FirebaseProjectID, cfg.Env == "local",
					withJobLogger(jobLogger, mux),
				),
			),
		),
	)

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Synthify API listening on %s", addr)
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
