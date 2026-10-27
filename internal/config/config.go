// Package config 对应 Python app/config.py：.env 加载 / 入站鉴权 / cookie 写回。
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LoadEnv 极简 .env：KEY=VALUE，# 注释，不覆盖已存在的环境变量
// （对齐 config.load_env 的 setdefault 语义）。path 为空时依次找
// DOUBAO_STATE_DIR、exe 目录、工作目录下的 .env。
func LoadEnv(path string) {
	if path == "" {
		for _, cand := range []string{
			os.Getenv("DOUBAO_STATE_DIR"),
			exeDir(),
			cwd(),
		} {
			if cand == "" {
				continue
			}
			p := filepath.Join(cand, ".env")
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
		if path == "" {
			return
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if _, ok := os.LookupEnv(k); !ok {
			os.Setenv(k, v)
		}
	}
}

// APIKey 入站鉴权密钥；空串 = 本地裸奔模式（对齐 config.api_key）。
func APIKey() string {
	return or(os.Getenv("API_KEY"), os.Getenv("DOUBAO_API_KEY"))
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// SaveCookie 把 DOUBAO_COOKIE 落盘到独立凭据文件 cookie.json（0600，原子替换），
// 不写 .env——凭据与配置分离。返回写入路径；调用方负责 os.Setenv 热更新。
func SaveCookie(cookie, path string) (string, error) {
	if path == "" {
		dir := os.Getenv("DOUBAO_STATE_DIR")
		if dir == "" {
			dir = exeDir()
		}
		path = filepath.Join(dir, "cookie.json")
	}
	b, err := json.MarshalIndent(map[string]string{"cookie": cookie, "saved_at": time.Now().Format(time.RFC3339)}, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	os.Setenv("DOUBAO_COOKIE", cookie)
	return path, nil
}

func exeDir() string {
	if p, err := os.Executable(); err == nil {
		return filepath.Dir(p)
	}
	return ""
}

func cwd() string {
	if p, err := os.Getwd(); err == nil {
		return p
	}
	return ""
}
