//go:build !desktop && !bindings

// Browser mode: a local HTTP server that opens the UI in the default browser.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8088", "listen address")
	cfg := flag.String("config", defaultConfig(), "clusters file")
	noOpen := flag.Bool("no-open", false, "do not open the browser")
	flag.Parse()

	store, err := OpenStore(*cfg)
	if err != nil {
		log.Fatalf("load %s: %v", *cfg, err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + ln.Addr().String()
	log.Printf("KafkaLens on %s (clusters: %s)", url, *cfg)
	if !*noOpen {
		openBrowser(url)
	}
	// no WriteTimeout: live tail is a long-lived SSE stream
	srv := &http.Server{Handler: newHandler(store), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	log.Fatal(srv.Serve(ln))
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
