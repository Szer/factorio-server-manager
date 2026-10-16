package api

import (
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettingsMutationBlockedByProcessLock(t *testing.T) {
	factorio.GameFilesMu.Lock()
	defer factorio.GameFilesMu.Unlock()
	called := false
	handler := ServerOffMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, httptest.NewRequest("POST", "/api/mods/startup-settings", nil))
	if result.Code != http.StatusLocked || called {
		t.Fatal("mutation ran while files were in use")
	}
}
