// Package config 对应 Python app/config.py：.env 加载 / 入站鉴权 / cookie 写回。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// SaveCookie 把 DOUBAO_COOKIE 写回 .env 并更新当前进程环境变量，返回写入路径
// （对齐 config.save_cookie：只改这一行，其余原样保留；凭据不落日志）。
func SaveCookie(cookie, path string) (string, error) {
	if path == "" {
		dir := os.Getenv("DOUBAO_STATE_DIR")
		if dir == "" {
			dir = exeDir()
		}
		path = filepath.Join(dir, ".env")
	}
	line := fmt.Sprintf("DOUBAO_COOKIE=%q", cookie)
	lines := []string{}
	if raw, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	}
	replaced := false
	for i, old := range lines {
		if strings.HasPrefix(strings.TrimSpace(old), "DOUBAO_COOKIE") {
			lines[i] = line
			replaced = true
			break
		}
	}
	if !replaced {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "# ---- 上游凭证（/login 扫码写回，也可手工粘贴） ----")
		lines = append(lines, line)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
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
