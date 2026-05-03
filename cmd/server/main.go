package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/synthify/backend/apps/api/internal/handler"
	"github.com/synthify/backend/apps/api/internal/service"
	"github.com/synthify/backend/apps/worker/pkg/worker"
	"github.com/synthify/backend/packages/shared/app"
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
	dispatcher := initDispatcher(cfg)

	workspaceService := service.NewWorkspaceService(store, store)
	documentService := service.NewDocumentService(store, store, app.NewDocumentSourceURLBuilder(cfg.InternalGCSUploadBase), dispatcher, notifier)
	itemService := service.NewItemService(store, store)

	treeHandler := handler.NewTreeHandler(store, store, store)
	jobHandler := handler.NewJobHandler(store, store, store)

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewWorkspaceServiceHandler(handler.NewWorkspaceHandler(workspaceService, store)))
	mux.Handle(treev1connect.NewDocumentServiceHandler(handler.NewDocumentHandler(documentService, store, store, app.NewDocumentUploadURLBuilder(cfg.GCSUploadURLBase))))
	mux.Handle(treev1connect.NewJobServiceHandler(jobHandler))
	mux.Handle(treev1connect.NewTreeServiceHandler(treeHandler))
	mux.Handle(treev1connect.NewItemServiceHandler(handler.NewItemHandler(itemService, store, store)))
	mux.HandleFunc("GET /tree/subtree", treeHandler.GetSubtreeHTTP)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	h := middleware.Recover(
		middleware.Logger(
			middleware.CORS(cfg.CORSAllowedOrigins,
				middleware.WithAuth(cfg.FirebaseProjectID,
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
