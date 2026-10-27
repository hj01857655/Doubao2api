// 面板静态页：内置一份 panel.html（//go:embed），状态目录里有同名文件时
// 优先用用户的——内置版只是让 /admin 开箱即用，不依赖手动安装步骤。
package handler

import (
	_ "embed"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"doubao2api/internal/config"
	"doubao2api/internal/doubao"
)

//go:embed panel.html
var panelBuiltin []byte

// servePanel 输出面板 HTML：状态目录 panel.html > 内置版。
func (s *Server) servePanel(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile(filepath.Join(stateDir(), "panel.html"))
	if err != nil || len(raw) == 0 {
		raw = panelBuiltin
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(raw)
}

// adminOverview 面板概览：登录态 / 模型档位 / 上游 / 是否启用入站鉴权。
// 档位来自本地映射表 doubao.MODES，不查上游——未配置 cookie 时也可用。
func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	if !s.G.APIKeyOK(r) {
		openaiError(w, "invalid api key", "auth_error", "invalid_api_key", http.StatusUnauthorized)
		return
	}
	names := make([]string, 0, len(doubao.MODES))
	for name := range doubao.MODES {
		names = append(names, name)
	}
	sort.Strings(names) // map 遍历无序，面板显示要稳定
	models := make([]any, 0, len(names))
	for _, name := range names {
		models = append(models, map[string]any{"id": name, "display": doubao.MODES[name].Display})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"credential": map[string]any{"valid": s.G.Client != nil},
		"models":     models,
		"upstream":   "https://" + doubao.HOST + doubao.COMPLETION_PATH,
		"local_auth": config.APIKey() != "",
	})
}

// stateDir 状态目录，与 config.LoadEnv / main.go 日志落盘同一优先序：
// DOUBAO_STATE_DIR > exe 目录（扫码写回的 .env 也在这里）。
func stateDir() string {
	if dir := os.Getenv("DOUBAO_STATE_DIR"); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}
