//go:build !desktop && !bindings

// Browser mode: a local HTTP server that opens the UI in the default browser.
package main

import (
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"kafkalens/internal/config"
	"kafkalens/internal/store"
)

func main() {
	settings := flag.String("settings", "config.yaml", "startup settings (YAML); if the default file is missing, built-in defaults apply")
	addr := flag.String("addr", "", "listen address (overrides server.addr)")
	clusters := flag.String("config", "", "clusters file (overrides clusters)")
	noOpen := flag.Bool("no-open", false, "do not open the browser (overrides server.openBrowser)")
	flag.Parse()

	// an explicit -settings file must exist; the default ./config.yaml is optional
	cfg, source, err := config.Load(defaultSettings, *settings, !flagSet("settings"))
	if err != nil {
		fatal("invalid settings", "file", source, "error", err)
	}
	if *addr != "" {
		cfg.Server.Addr = *addr
	}
	if *clusters != "" {
		cfg.Clusters = *clusters
	}
	if *noOpen {
		cfg.Server.OpenBrowser = false
	}
	setupLogging(os.Stderr, cfg) // browser mode runs from a terminal: log to the console
	cfgPath := clustersFile(cfg)

	st, err := store.OpenStore(cfgPath)
	if err != nil {
		fatal("load clusters", "file", cfgPath, "error", err)
	}
	ln, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		fatal("listen", "addr", cfg.Server.Addr, "error", err)
	}
	url := "http://" + ln.Addr().String()
	slog.Info("KafkaLens started", "version", version(), "mode", "browser", "url", url, "settings", source, "clusters", cfgPath)
	if cfg.Server.OpenBrowser {
		openBrowser(url)
	}
	// no WriteTimeout: live tail is a long-lived SSE stream
	srv := &http.Server{Handler: newHandler(st), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	fatal("server stopped", "error", srv.Serve(ln))
}

// flagSet reports whether the named flag was given on the command line.
func flagSet(name string) (set bool) {
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case os.Getenv("WSL_DISTRO_NAME") != "":
		cmd = exec.Command("cmd.exe", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
