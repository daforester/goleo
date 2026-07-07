package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the Goleo development server",
	Long: `Starts the Goleo development environment with hot reload.

Starts both:
  - The Go backend server with live reload
  - The Vite frontend dev server with HMR

The frontend dev server proxies API calls to the Go backend.`,
	RunE: runDev,
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
	frontendPath := frontendDir
	if !filepath.IsAbs(frontendPath) {
		frontendPath = filepath.Join(".", frontendPath)
	}

	frontendAbs, err := filepath.Abs(frontendPath)
	if err != nil {
		return fmt.Errorf("invalid frontend path: %w", err)
	}

	if _, err := os.Stat(filepath.Join(frontendAbs, "package.json")); os.IsNotExist(err) {
		return fmt.Errorf("frontend directory not found at %s", frontendAbs)
	}

	envVars := []string{
		fmt.Sprintf("GOLEO_DEV=true"),
		fmt.Sprintf("GOLEO_PORT=%d", devPort),
		fmt.Sprintf("GOLEO_FRONTEND_DIR=%s", frontendAbs),
	}

	goBackend := exec.Command("go", "run", ".")
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
		goBackend.Process.Kill()
		return fmt.Errorf("failed to start Vite dev server: %w", err)
	}

	done := make(chan error, 2)
	go func() { done <- goBackend.Wait() }()
	go func() { done <- viteCmd.Wait() }()

	err = <-done
	goBackend.Process.Kill()
	viteCmd.Process.Kill()
	return err
}
