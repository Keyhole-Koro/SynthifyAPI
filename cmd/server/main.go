package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	graphv1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1/graphv1connect"
	"github.com/synthify/backend/api/internal/handler"
	"github.com/synthify/backend/api/internal/service"
	"github.com/synthify/backend/internal/app"
	"github.com/synthify/backend/internal/jobstatus"
	"github.com/synthify/backend/internal/middleware"
	"github.com/synthify/backend/internal/worker"
)

func main() {
	ctx := context.Background()
	port := envOrDefault("PORT", "8080")
	corsOrigins := envOrDefault("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
	uploadURLBase := envOrDefault("GCS_UPLOAD_URL_BASE", "http://localhost:4443/synthify-uploads")
	internalUploadURLBase := envOrDefault("INTERNAL_GCS_UPLOAD_URL_BASE", uploadURLBase)

	store := app.InitStore(ctx, app.PublicUploadURLGenerator(uploadURLBase))
	dispatcher := initDispatcher()
	notifier := jobstatus.NewNotifier(ctx, os.Getenv("FIREBASE_PROJECT_ID"))

	workspaceService := service.NewWorkspaceService(store, store)
	documentService := service.NewDocumentService(store, store, app.PublicUploadURLGenerator(internalUploadURLBase), dispatcher, notifier)
	graphService := service.NewGraphService(store)
	nodeService := service.NewNodeService(store, store)

	mux := http.NewServeMux()
	mux.Handle(graphv1connect.NewWorkspaceServiceHandler(handler.NewWorkspaceHandler(workspaceService)))
	mux.Handle(graphv1connect.NewDocumentServiceHandler(handler.NewDocumentHandler(documentService, store, store, app.PublicUploadURLGenerator(uploadURLBase))))
	mux.Handle(graphv1connect.NewGraphServiceHandler(handler.NewGraphHandler(graphService, store, store)))
	mux.Handle(graphv1connect.NewNodeServiceHandler(handler.NewNodeHandler(nodeService, store, store)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	h := middleware.Logger(middleware.CORS(corsOrigins, middleware.WithAuth(os.Getenv("FIREBASE_PROJECT_ID"), mux)))
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Synthify API listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

func initDispatcher() worker.Dispatcher {
	if baseURL := os.Getenv("WORKER_BASE_URL"); baseURL != "" {
		return worker.NewHTTPDispatcher(baseURL, os.Getenv("INTERNAL_WORKER_TOKEN"))
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
