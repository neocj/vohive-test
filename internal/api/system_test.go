package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDisabledSystemEndpointsReturnGone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}

	tests := []struct {
		name    string
		method  string
		path    string
		handler func(*gin.Context)
		code    string
	}{
		{
			name:    "check update",
			method:  http.MethodGet,
			path:    "/api/system/update/check",
			handler: s.handleCheckUpdate,
			code:    "online_update_disabled",
		},
		{
			name:    "apply update",
			method:  http.MethodPost,
			path:    "/api/system/update/apply",
			handler: s.handleApplyUpdate,
			code:    "online_update_disabled",
		},
		{
			name:    "uninstall",
			method:  http.MethodPost,
			path:    "/api/system/uninstall",
			handler: s.handleUninstall,
			code:    "uninstall_disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(tt.method, tt.path, nil)

			tt.handler(c)

			if w.Code != http.StatusGone {
				t.Fatalf("code=%d body=%s, want 410", w.Code, w.Body.String())
			}
			if !containsJSONField(w.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("body=%s, want code %q", w.Body.String(), tt.code)
			}
		})
	}
}

func TestRouterDoesNotRegisterUpdateOrUninstallEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}
	r := s.newRouter()

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/system/update/check"},
		{method: http.MethodPost, path: "/api/system/update/apply"},
		{method: http.MethodPost, path: "/api/system/uninstall"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)

			r.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Fatalf("code=%d body=%s, want 404", w.Code, w.Body.String())
			}
		})
	}
}

func containsJSONField(body string, field string) bool {
	for i := 0; i+len(field) <= len(body); i++ {
		if body[i:i+len(field)] == field {
			return true
		}
	}
	return false
}
