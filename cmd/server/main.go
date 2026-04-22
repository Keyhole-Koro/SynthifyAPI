package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/app"
	"github.com/Keyhole-Koro/SynthifyShared/config"
	graphv1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1/graphv1connect"
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
	graphService := service.NewGraphService(store)
	nodeService := service.NewNodeService(store, store)

	graphHandler := handler.NewGraphHandler(graphService, store, store)

	mux := http.NewServeMux()
	mux.Handle(graphv1connect.NewWorkspaceServiceHandler(handler.NewWorkspaceHandler(workspaceService)))
	mux.Handle(graphv1connect.NewDocumentServiceHandler(handler.NewDocumentHandler(documentService, store, store, app.PublicUploadURLGenerator(cfg.GCSUploadURLBase))))
	mux.Handle(graphv1connect.NewJobServiceHandler(handler.NewJobHandler(jobService, store, store)))
	mux.Handle(graphv1connect.NewGraphServiceHandler(graphHandler))
	mux.Handle(graphv1connect.NewNodeServiceHandler(handler.NewNodeHandler(nodeService, store, store)))
	mux.HandleFunc("GET /graph/subtree", graphHandler.GetSubtreeHTTP)
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

func initDispatcher(cfg config.API) worker.Dispatcher {
	if cfg.WorkerBaseURL != "" {
		return worker.NewHTTPDispatcher(cfg.WorkerBaseURL, cfg.InternalWorkerToken)
	}
	return nil
}
