package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kevinyuzekai/ComputePool/internal/api"
	"github.com/kevinyuzekai/ComputePool/internal/hub"
	"github.com/kevinyuzekai/ComputePool/internal/localworker"
	"github.com/kevinyuzekai/ComputePool/internal/mdns"
	"github.com/kevinyuzekai/ComputePool/internal/paths"
	"github.com/kevinyuzekai/ComputePool/web"
)

// Set via -ldflags "-X main.version=…"
var version = "0.2.0"

func main() {
	listen := flag.String("listen", hub.DefaultListen, "Hub 监听地址（LAN，默认 0.0.0.0:9797）")
	workers := flag.Int("local-workers", 0, "本机 worker 协程数（0 = NumCPU）")
	inbox := flag.String("inbox", "", "图片 Inbox 目录（默认 ~/ComputePool-Inbox）")
	outbox := flag.String("outbox", "", "图片 Outbox 目录（默认 ~/ComputePool-Outbox）")
	openFlag := flag.Bool("open", false, "启动后打开浏览器")
	noMDNS := flag.Bool("no-mdns", false, "禁用 Bonjour/mDNS 广播")
	showVer := flag.Bool("version", false, "打印版本")
	flag.Parse()

	if *showVer {
		fmt.Println("ComputePool", version)
		return
	}

	autoOpen := *openFlag || runningFromAppBundle()

	h := hub.New(version)
	h.SetListenAddr(*listen)
	inDir, outDir := *inbox, *outbox
	if inDir == "" {
		inDir = paths.DefaultInbox()
	}
	if outDir == "" {
		outDir = paths.DefaultOutbox()
	}
	if err := h.SetDirs(inDir, outDir); err != nil {
		log.Fatalf("inbox/outbox: %v", err)
	}
	h.Start()

	pool := &localworker.Pool{Hub: h, Count: *workers}
	pool.Start()

	mux := http.NewServeMux()
	apiHandler := &api.Handler{Hub: h}
	apiHandler.Mount(mux)

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := fs.ReadFile(web.Static, "static/index.html")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})

	server := &http.Server{Addr: *listen, Handler: cors(mux)}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listen %s: %v", *listen, err)
	}

	info := h.Info()
	var adv mdns.Advertiser
	if !*noMDNS {
		_ = adv.Start(info.Port, info.JoinURL)
	}

	go func() {
		log.Printf("ComputePool %s hub → http://%s  (join %s)", version, *listen, info.JoinURL)
		log.Printf("本机 local workers: %d · 协议 _computepool._tcp", pool.Count)
		ipaths := h.ImagePaths()
		log.Printf("图片 Inbox: %s", ipaths.Inbox)
		log.Printf("图片 Outbox: %s", ipaths.Outbox)
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	uiURL := fmt.Sprintf("http://127.0.0.1:%d", info.Port)
	if autoOpen {
		if err := openBrowser(uiURL); err != nil {
			log.Printf("打开浏览器失败: %v — 请手动打开 %s", err, uiURL)
		}
		if runningFromAppBundle() {
			notifyUIReady(uiURL)
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("正在停止…")
	adv.Stop()
	pool.Stop()
	h.Stop()
	_ = server.Close()
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func runningFromAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return strings.Contains(exe, ".app/Contents/MacOS/")
}
