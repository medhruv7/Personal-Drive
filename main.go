// Command localdrive serves a folder on this machine as a personal web drive:
// browse, upload, download and create folders from any browser on the network.
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

//go:embed web
var webFiles embed.FS

func main() {
	home, _ := os.UserHomeDir()
	var (
		rootDir    = flag.String("root", envOr("LOCALDRIVE_ROOT", filepath.Join(home, "LocalDrive")), "folder to serve (env LOCALDRIVE_ROOT)")
		addr       = flag.String("addr", envOr("LOCALDRIVE_ADDR", ":8080"), "listen address (env LOCALDRIVE_ADDR)")
		showHidden = flag.Bool("show-hidden", envBool("LOCALDRIVE_SHOW_HIDDEN"), "show dotfiles and system files (env LOCALDRIVE_SHOW_HIDDEN)")
		maxUpload  = flag.Int64("max-upload", envInt("LOCALDRIVE_MAX_UPLOAD"), "max bytes per upload request, 0 = unlimited (env LOCALDRIVE_MAX_UPLOAD)")
	)
	flag.Parse()

	absRoot, err := filepath.Abs(*rootDir)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		log.Fatalf("create root folder: %v", err)
	}
	root, err := os.OpenRoot(absRoot)
	if err != nil {
		log.Fatalf("open root folder: %v", err)
	}
	defer root.Close()

	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	s := &Server{
		root:       root,
		rootName:   filepath.Base(absRoot),
		showHidden: *showHidden,
		maxUpload:  *maxUpload,
		static:     static,
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No ReadTimeout/WriteTimeout: they would cut off large transfers.
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	printBanner(absRoot, ln.Addr().(*net.TCPAddr).Port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func printBanner(root string, port int) {
	fmt.Printf("\nLocal Drive is serving: %s\n\n", root)
	fmt.Printf("  On this computer:  http://localhost:%d\n", port)
	for _, ip := range lanIPs() {
		fmt.Printf("  On your network:   http://%s:%d\n", ip, port)
	}
	fmt.Println("\n  WARNING: no authentication — anyone who can reach this port can read and write the folder.")
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()
}

// lanIPs returns the non-loopback IPv4 addresses of interfaces that are up.
func lanIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ip4 := ipnet.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() {
					ips = append(ips, ip4.String())
				}
			}
		}
	}
	return ips
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	b, _ := strconv.ParseBool(os.Getenv(key))
	return b
}

func envInt(key string) int64 {
	n, _ := strconv.ParseInt(os.Getenv(key), 10, 64)
	return n
}
