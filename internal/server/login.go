package server

import (
	_ "embed"
	"html/template"
	"net/http"
)

//go:embed login.html
var loginHTML string

var loginTemplate = template.Must(template.New("login").Parse(loginHTML))

func (a *Editor) loginPage(w http.ResponseWriter, status int, code string) {
	message := map[string]string{
		"login_failed":                  "登录未完成，请重试。",
		"access_denied":                 "当前身份没有管理权限，请联系管理员或使用获授权的身份登录。",
		"login_rate_limited":            "登录尝试过于频繁，请稍后重试。",
		"login_temporarily_unavailable": "暂时无法连接登录服务，请稍后重试。",
		"login_unavailable":             "暂时无法完成登录，请稍后重试。",
	}[code]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = loginTemplate.Execute(w, struct {
		Error string
	}{message})
}
