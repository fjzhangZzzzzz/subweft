package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginPresentation(t *testing.T) {
	for _, tt := range []struct{ code, message string }{
		{"login_failed", "登录未完成，请重试。"},
		{"login_temporarily_unavailable", "暂时无法连接登录服务，请稍后重试。"},
	} {
		t.Run(tt.code, func(t *testing.T) {
			t.Setenv("SUBWEFT_OIDC_DISPLAY_NAME", "Pocket ID")
			w := httptest.NewRecorder()
			(&Editor{}).loginPage(w, 401, tt.code)
			body := w.Body.String()
			if w.Code != 401 || !strings.Contains(body, "继续登录") || !strings.Contains(body, tt.message) || !strings.Contains(body, `href="/auth/login"`) || strings.Contains(body, "Pocket ID") || !strings.Contains(body, "将跳转至登录服务，完成后返回 Subweft") {
				t.Fatal("login response lost status, safe text, or retry link")
			}
		})
	}
}
