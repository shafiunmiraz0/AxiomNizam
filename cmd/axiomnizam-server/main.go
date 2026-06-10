package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/axiomnizam/internal/apiserver"
	"example.com/axiomnizam/internal/cache"
	"example.com/axiomnizam/internal/controllers"
	"example.com/axiomnizam/internal/events"
)

var (
	port        = flag.Int("port", 8000, "Server port")
	environment = flag.String("env", "development", "Environment: development, staging, production")
)

func main() {
	flag.Parse()

	fmt.Printf("🚀 Starting AxiomNizam Platform (v1.0.0)\n")
	fmt.Printf("📍 Environment: %s\n", *environment)
	fmt.Printf("🔌 Port: %d\n", *port)
	fmt.Println()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize infrastructure
	fmt.Println("⚙️  Initializing infrastructure...")

	// Initialize cache
	cacheStore := cache.NewThreadSafeStore()
	fmt.Println("  ✓ Cache initialized")

	// Initialize informer factory
	informerFactory := cache.NewInformerFactory()
	fmt.Println("  ✓ Informer factory created")

	// Initialize event bus
	eventBus := events.NewMemoryBus(10000)
	fmt.Println("  ✓ Event bus created")

	// Initialize event recorder
	eventRecorder := events.NewEventRecorder()
	_ = eventBus // event bus available for subscriptions
	fmt.Println("  ✓ Event recorder created")

	// Initialize controller manager
	controllerManager := controllers.NewControllerManager(informerFactory)
	fmt.Println("  ✓ Controller manager created")

	// Start controller manager
	fmt.Println("\n🎮 Starting controllers...")
	if err := controllerManager.Start(ctx); err != nil {
		log.Fatalf("Failed to start controller manager: %v", err)
	}
	fmt.Println("  ✓ Controllers started")

	// Wait for controllers to sync
	syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := controllerManager.WaitForSync(syncCtx); err != nil {
		log.Fatalf("Controllers failed to sync: %v", err)
	}
	syncCancel()
	fmt.Println("  ✓ All controllers synced")

	// Initialize API server
	fmt.Println("\n📡 Starting API server...")
	apiStore := apiserver.NewResourceStore()
	apiSrv := apiserver.NewAPIServer(apiStore)
	apiSrv.RegisterRoutes()
	_ = controllerManager // controller manager available for reconciliation
	_ = eventRecorder     // event recorder available for audit
	_ = cacheStore        // cache store available for caching
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      apiSrv.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		fmt.Printf("✅ API Server listening on http://localhost:%d\n", *port)
		fmt.Println("\n📚 API Documentation: http://localhost:" + fmt.Sprintf("%d", *port) + "/api/docs")
		fmt.Println("🏥 Health Check: http://localhost:" + fmt.Sprintf("%d", *port) + "/health")
		fmt.Println("\n🎯 Ready to accept requests...")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	<-sigCh
	fmt.Println("\n\n🛑 Shutdown signal received, gracefully stopping...")

	// Shutdown server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	fmt.Println("  ✓ API server stopped")

	// Stop controllers
	if err := controllerManager.Stop(shutdownCtx); err != nil {
		log.Printf("Controller manager stop error: %v", err)
	}
	fmt.Println("  ✓ Controllers stopped")

	fmt.Println("\n✨ AxiomNizam server stopped cleanly")
}
