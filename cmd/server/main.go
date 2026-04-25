package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/app"
	"github.com/Keyhole-Koro/SynthifyShared/config"
	treev1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1/treev1connect"
	"github.com/Keyhole-Koro/SynthifyShared/jobstatus"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/synthify/backend/api/internal/handler"
	"github.com/synthify/backend/api/internal/service"
	"github.com/synthify/backend/worker/pkg/worker"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadAPI()

	store := app.InitStore(ctx, app.PublicUploadURLGenerator(cfg.GCSUploadURLBase))
	dispatcher := initDispatcher(cfg)
	notifier := jobstatus.NewNotifier(ctx, cfg.FirebaseProjectID)

	workspaceService := service.NewWorkspaceService(store, store)
	documentService := service.NewDocumentService(store, store, app.PublicUploadURLGenerator(cfg.InternalGCSUploadBase), dispatcher, notifier)
	jobService := service.NewJobService(store)
	treeService := service.NewTreeService(store)
	itemService := service.NewItemService(store, store)

	treeHandler := handler.NewTreeHandler(treeService, store, store)

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewWorkspaceServiceHandler(handler.NewWorkspaceHandler(workspaceService)))
	mux.Handle(treev1connect.NewDocumentServiceHandler(handler.NewDocumentHandler(documentService, store, store, app.PublicUploadURLGenerator(cfg.GCSUploadURLBase))))
	mux.Handle(treev1connect.NewJobServiceHandler(handler.NewJobHandler(jobService, store, store)))
	mux.Handle(treev1connect.NewTreeServiceHandler(treeHandler))
	mux.Handle(treev1connect.NewItemServiceHandler(handler.NewItemHandler(itemService, store, store)))
	mux.HandleFunc("GET /tree/subtree", treeHandler.GetSubtreeHTTP)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	h := middleware.Logger(middleware.CORS(cfg.CORSAllowedOrigins, middleware.WithAuth(cfg.FirebaseProjectID, mux)))
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Synthify API listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

func initDispatcher(cfg config.API) service.WorkerDispatcher {
	if cfg.WorkerBaseURL != "" {
		return worker.NewHTTPDispatcher(cfg.WorkerBaseURL, cfg.InternalWorkerToken)
	}
	return nil
}
