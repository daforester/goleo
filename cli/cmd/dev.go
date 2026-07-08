package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Start the Goleo development server",
	Long: `Starts the Go backend and Vite frontend dev server with HMR.

Subcommands:
  pwa   Start Vite dev server only (no Go backend)`,
	RunE: runDev,
}

var devPwaCmd = &cobra.Command{
	Use:   "pwa",
	Short: "Start PWA development server (frontend only)",
	Long: `Starts the Vite dev server with HMR for PWA development, without a Go backend.

Frontend changes reflect instantly via HMR. No Go backend is started.
Useful for testing the frontend in a browser or on a mobile device.`,
	RunE: runDevPWA,
}

var (
	devPort     int
	frontendDir string
)

func init() {
	devCmd.Flags().IntVarP(&devPort, "port", "p", 9842, "Port for the Go backend server")
	devCmd.Flags().StringVarP(&frontendDir, "frontend-dir", "f", "frontend", "Path to frontend directory")
	devCmd.AddCommand(devPwaCmd)
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

	if err := generateBackendEntrypoints("."); err != nil {
		return fmt.Errorf("generating backend entry points: %w", err)
	}

	if err := checkPortAvailable(devPort); err != nil {
		return err
	}

	// Make sure cgo can find an installed WebKitGTK before compiling the
	// backend, which embeds the webview runtime. On distros that ship only
	// webkit2gtk-4.1 this points pkg-config at the version that's present.
	webkitEnv, err := prepareWebkitEnv()
	if err != nil {
		return err
	}

	goBackend := exec.Command("go", "run", backendPkgDir())
	goBackend.Env = append(os.Environ(), envVars...)
	goBackend.Env = append(goBackend.Env, webkitEnv...)
	goBackend.Stdout = os.Stdout
	goBackend.Stderr = os.Stderr

	if err := goBackend.Start(); err != nil {
		return fmt.Errorf("failed to start Go backend: %w", err)
	}
	if err := bindProcessLifetime(goBackend); err != nil {
		fmt.Printf("  Warning: could not set up process cleanup safeguard: %v\n", err)
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
	if err := bindProcessLifetime(viteCmd); err != nil {
		fmt.Printf("  Warning: could not set up process cleanup safeguard: %v\n", err)
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

// checkPortAvailable fails fast with an actionable message if port is
// already bound, instead of letting the Go backend silently fall back to a
// random port (see runtime/server.go) — Vite's dev proxy targets this exact
// port, so a silent fallback means requests quietly go to whatever old
// process is squatting on it (typically an orphaned backend.exe/backend
// from a goleo dev session that didn't shut down cleanly) rather than the
// one just started.
func checkPortAvailable(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		killCmd := "pkill -f 'go-build.*/backend' (or lsof -i :" + fmt.Sprint(port) + " to find the exact PID)"
		if runtime.GOOS == "windows" {
			killCmd = "Get-Process backend -ErrorAction SilentlyContinue | Stop-Process -Force"
		}
		return fmt.Errorf(
			"port %d is already in use — likely a leftover backend process from a goleo dev session that wasn't cleanly stopped.\n"+
				"  Stop it first:\n    %s\n"+
				"  Then run goleo dev again", port, killCmd)
	}
	ln.Close()
	return nil
}

func runDevPWA(cmd *cobra.Command, args []string) error {
	frontendAbs, err := filepath.Abs(frontendDir)
	if err != nil {
		return fmt.Errorf("invalid frontend path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(frontendAbs, "package.json")); os.IsNotExist(err) {
		return fmt.Errorf("frontend directory not found at %s", frontendAbs)
	}

	if _, err := os.Stat(filepath.Join(frontendAbs, "node_modules")); os.IsNotExist(err) {
		fmt.Println("  Installing frontend dependencies...")
		install := exec.Command("npm", "install")
		install.Dir = frontendAbs
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		if err := install.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
	}

	viteCmd := exec.Command("npx", "vite", "--port", "5173", "--host")
	viteCmd.Dir = frontendAbs
	viteCmd.Env = append(os.Environ(), "VITE_GOLEO_PLATFORM=pwa")
	viteCmd.Stdout = os.Stdout
	viteCmd.Stderr = os.Stderr

	if err := viteCmd.Start(); err != nil {
		return fmt.Errorf("failed to start Vite dev server: %w", err)
	}

	fmt.Println("  Starting Goleo PWA development server...")
	fmt.Printf("  Frontend: http://localhost:5173\n")
	fmt.Println()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- viteCmd.Wait() }()

	select {
	case sig := <-sigCh:
		fmt.Printf("\n  Received %s, shutting down...\n", sig)
	case err = <-done:
	}

	killProcTree(viteCmd.Process.Pid)
	viteCmd.Wait()

	fmt.Println("  All processes stopped.")
	return err
}
