package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginPresentation(t *testing.T) {
	for _, tt := range []struct{ name, label string }{
		{"", "继续登录"},
		{" Pocket ID ", "使用 Pocket ID 登录"},
		{"<script>alert(1)</script>", "使用 &lt;script&gt;alert(1)&lt;/script&gt; 登录"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			(&Editor{ProviderName: tt.name}).loginPage(w, 401, "login_failed")
			body := w.Body.String()
			if w.Code != 401 || !strings.Contains(body, tt.label) || !strings.Contains(body, "登录未完成，请重试。") || !strings.Contains(body, `href="/auth/login"`) || strings.Contains(body, "<script>") {
				t.Fatal("login response lost status, safe text, or retry link")
			}
		})
	}
}
