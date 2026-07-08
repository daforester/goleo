package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the Goleo development server",
	Long:  `Starts the Go backend and Vite frontend dev server with HMR.`,
	RunE:  runDev,
}

var (
	devPort     int
	frontendDir string
)

func init() {
	devCmd.Flags().IntVarP(&devPort, "port", "p", 9842, "Port for the Go backend server")
	devCmd.Flags().StringVarP(&frontendDir, "frontend-dir", "f", "frontend", "Path to frontend directory")
}

func runDev(cmd *cobra.Command, args []string) error {
	frontendAbs, err := filepath.Abs(frontendDir)
	if err != nil {
		return fmt.Errorf("invalid frontend path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(frontendAbs, "package.json")); os.IsNotExist(err) {
		return fmt.Errorf("frontend directory not found at %s", frontendAbs)
	}

	envVars := []string{
		"GOLEO_DEV=true",
		fmt.Sprintf("GOLEO_PORT=%d", devPort),
		fmt.Sprintf("GOLEO_FRONTEND_DIR=%s", frontendAbs),
	}

	fmt.Println("  Resolving Go dependencies...")
	if err := ensureLocalReplace("."); err != nil {
		return fmt.Errorf("go module resolution: %w", err)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %w", err)
	}

	goBackend := exec.Command("go", "run", backendPkgDir())
	goBackend.Env = append(os.Environ(), envVars...)
	goBackend.Stdout = os.Stdout
	goBackend.Stderr = os.Stderr

	if err := goBackend.Start(); err != nil {
		return fmt.Errorf("failed to start Go backend: %w", err)
	}

	viteCmd := exec.Command("npx", "vite", "--port", "5173")
	viteCmd.Dir = frontendAbs
	viteCmd.Stdout = os.Stdout
	viteCmd.Stderr = os.Stderr

	fmt.Println("  Starting Goleo development server...")
	fmt.Printf("  Frontend: http://localhost:5173\n")
	fmt.Printf("  Backend:  http://localhost:%d\n", devPort)
	fmt.Println()

	if err := viteCmd.Start(); err != nil {
		killProcTree(goBackend.Process.Pid)
		return fmt.Errorf("failed to start Vite dev server: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan error, 2)
	go func() { done <- goBackend.Wait() }()
	go func() { done <- viteCmd.Wait() }()

	select {
	case sig := <-sigCh:
		fmt.Printf("\n  Received %s, shutting down...\n", sig)
	case err = <-done:
	}

	killProcTree(goBackend.Process.Pid)
	killProcTree(viteCmd.Process.Pid)

	goBackend.Wait()
	viteCmd.Wait()

	fmt.Println("  All processes stopped.")
	return err
}
