package serverboot

import (
	"example.com/axiomnizam/internal/logging"
	"context"
	"fmt"
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

// Run starts the API server runtime with graceful shutdown.
func Run(port int, environment string) error {
	fmt.Printf("🚀 Starting AxiomNizam Platform (v1.0.0)\n")
	fmt.Printf("📍 Environment: %s\n", environment)
	fmt.Printf("🔌 Port: %d\n", port)
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("⚙️  Initializing infrastructure...")

	cacheStore := cache.NewThreadSafeStore()
	fmt.Println("  ✓ Cache initialized")

	informerFactory := cache.NewInformerFactory()
	fmt.Println("  ✓ Informer factory created")

	eventBus := events.NewMemoryBus(10000)
	fmt.Println("  ✓ Event bus created")

	eventRecorder := events.NewEventRecorder()
	_ = eventBus // available for subscriptions
	fmt.Println("  ✓ Event recorder created")

	controllerManager := controllers.NewControllerManager(informerFactory)
	fmt.Println("  ✓ Controller manager created")

	fmt.Println("\n🎮 Starting controllers...")
	if err := controllerManager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start controller manager: %w", err)
	}
	fmt.Println("  ✓ Controllers started")

	syncCtx, syncCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := controllerManager.WaitForSync(syncCtx); err != nil {
		syncCancel()
		return fmt.Errorf("controllers failed to sync: %w", err)
	}
	syncCancel()
	fmt.Println("  ✓ All controllers synced")

	fmt.Println("\n📡 Starting API server...")
	apiStore := apiserver.NewResourceStore()
	apiSrv := apiserver.NewAPIServer(apiStore)
	apiSrv.RegisterRoutes()
	_ = controllerManager
	_ = eventRecorder
	_ = cacheStore

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      apiSrv.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		fmt.Printf("✅ API Server listening on http://localhost:%d\n", port)
		fmt.Println("\n📚 API Documentation: http://localhost:" + fmt.Sprintf("%d", port) + "/api/docs")
		fmt.Println("🏥 Health Check: http://localhost:" + fmt.Sprintf("%d", port) + "/health")
		fmt.Println("\n🎯 Ready to accept requests...")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Z().Fatal(fmt.Sprintf("Server error: %v", err))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n\n🛑 Shutdown signal received, gracefully stopping...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logging.Z().Info(fmt.Sprintf("Server shutdown error: %v", err))
	}
	fmt.Println("  ✓ API server stopped")

	if err := controllerManager.Stop(shutdownCtx); err != nil {
		logging.Z().Info(fmt.Sprintf("Controller manager stop error: %v", err))
	}
	fmt.Println("  ✓ Controllers stopped")

	fmt.Println("\n✨ AxiomNizam server stopped cleanly")
	return nil
}
