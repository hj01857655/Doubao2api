// Doubao2api 入口（对齐 Python main.py 的 __main__ 段：无子命令，
// 装配 config → CLIENT → app，监听 127.0.0.1）。
package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"doubao2api/internal/config"
	"doubao2api/internal/doubao"
	"doubao2api/internal/gateway"
	"doubao2api/internal/handler"
)

func main() {
	// .env 极简加载：不覆盖已存在的环境变量（对齐 config.load_env）
	config.LoadEnv(os.Getenv("DOUBAO_ENV_PATH"))

	log := newLogger()

	// 上游客户端：cookie 缺失/不完整时照常起服务，数据面请求回 503
	// （对齐 gateway.py 的 CLIENT 构造失败置 None + main.py 的提示横幅）
	var client *doubao.Client
	if cookie := os.Getenv("DOUBAO_COOKIE"); cookie == "" {
		fmt.Println(strings.Repeat("=", 62))
		fmt.Println("  未检测到 DOUBAO_COOKIE：服务照常启动，但对话请求将返回 503")
		fmt.Println("  需要以下 10 个 cookie 名（/login 扫码可自动写入 .env）：")
		fmt.Println("    " + strings.Join(doubao.REQUIRED_COOKIES, ", "))
		fmt.Println(strings.Repeat("=", 62))
	} else {
		c, err := doubao.NewClient(cookie, 0, log)
		if err != nil {
			log.Error("启动即失败：" + err.Error())
		} else {
			client = c
		}
	}

	g := gateway.New(client, log)
	srv := handler.NewServer(g, log)

	port := orDefault(os.Getenv("PORT"), "8378")
	addr := "127.0.0.1:" + port

	var names []string
	for name := range doubao.MODES {
		names = append(names, name)
	}
	fmt.Println(strings.Repeat("=", 62))
	fmt.Println("  Doubao2api (Go) 已启动: http://" + addr)
	fmt.Println("  可用模型: " + strings.Join(names, ", "))
	fmt.Println("  OpenAI Base URL: http://" + addr + "/v1")
	if config.APIKey() != "" {
		fmt.Println("  鉴权: 已启用 API_KEY（Bearer 或 x-api-key）")
	} else {
		fmt.Println("  鉴权: 未设置 API_KEY（本地裸奔模式）")
	}
	if client != nil {
		fmt.Println("  上游: https://" + doubao.HOST + doubao.COMPLETION_PATH + "（cookie 已就绪）")
	} else {
		fmt.Println("  上游: 未就绪（/health 的 ok=false；扫码登录后自动热重建）")
	}
	fmt.Println("按 Ctrl+C 停止")

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "监听失败:", err)
		os.Exit(1)
	}
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// newLogger slog：stdout + server.log（2MB 轮转保 3 份，对齐 config.setup_logging）。
// 落盘位置：DOUBAO_STATE_DIR（默认 exe 目录；冒烟时指临时目录，不污染源码树）。
func newLogger() *slog.Logger {
	dir := os.Getenv("DOUBAO_STATE_DIR")
	if dir == "" {
		if exe, err := os.Executable(); err == nil {
			dir = filepath.Dir(exe)
		} else {
			dir = "."
		}
	}
	path := filepath.Join(dir, "server.log")
	rw := &rotateWriter{path: path, maxBytes: 2 << 20, backupCount: 3}
	return slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, rw), nil))
}

// rotateWriter 2MB×3 轮转（与 CodeArts2api-go 同款实现）。
type rotateWriter struct {
	mu          sync.Mutex
	file        *os.File
	written     int64
	path        string
	maxBytes    int64
	backupCount int
}

func (rw *rotateWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.file == nil {
		if err := rw.open(); err != nil {
			return 0, err
		}
	}
	if rw.written+int64(len(p)) > rw.maxBytes {
		rw.rotate()
	}
	n, err := rw.file.Write(p)
	rw.written += int64(n)
	return n, err
}

func (rw *rotateWriter) open() error {
	f, err := os.OpenFile(rw.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	rw.file = f
	if info, err := f.Stat(); err == nil {
		rw.written = info.Size()
	}
	return nil
}

func (rw *rotateWriter) rotate() {
	if rw.file != nil {
		rw.file.Close()
		rw.file = nil
	}
	for i := rw.backupCount - 1; i > 0; i-- {
		os.Rename(fmt.Sprintf("%s.%d", rw.path, i), fmt.Sprintf("%s.%d", rw.path, i+1))
	}
	os.Rename(rw.path, rw.path+".1")
	rw.written = 0
	_ = rw.open()
}
