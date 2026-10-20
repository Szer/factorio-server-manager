package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/factorio"
)

func TestSettingsSaveFailures(t *testing.T) {
	original := bootstrap.GetConfig()
	previous := *factorio.GetFactorioServer()
	t.Cleanup(func() {
		bootstrap.NewConfig([]string{"--conf", original.ConfFile, "--dir", original.FactorioDir, "--mod-pack-dir", original.FactorioModPackDir})
		factorio.SetFactorioServer(previous)
	})
	for _, scenario := range []string{"settings-failure", "admins-failure", "success", "invalid-json", "null"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			config := original
			config.SettingsFile = filepath.Join(dir, "server-settings.json")
			config.FactorioAdminFile = filepath.Join(dir, "server-adminlist.json")
			conf := filepath.Join(dir, "conf.json")
			data, _ := json.Marshal(config)
			if err := os.WriteFile(conf, data, 0600); err != nil {
				t.Fatal(err)
			}
			bootstrap.NewConfig([]string{"--conf", conf, "--dir", original.FactorioDir, "--mod-pack-dir", original.FactorioModPackDir})
			factorio.SetFactorioServer(factorio.Server{Version: factorio.Version{2, 1, 23, 0}, Settings: map[string]interface{}{"name": "old", "preserved": true}})
			if scenario == "settings-failure" {
				if err := os.Mkdir(config.SettingsFile, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "admins-failure" {
				if err := os.Mkdir(config.FactorioAdminFile, 0700); err != nil {
					t.Fatal(err)
				}
			}
			body := `{"name":"new","admins":["admin"]}`
			if scenario == "invalid-json" {
				body = `{"name":"new",`
			}
			if scenario == "null" {
				body = `null`
			}
			response := httptest.NewRecorder()
			UpdateServerSettings(response, httptest.NewRequest(http.MethodPost, "/api/settings/update", strings.NewReader(body)))
			expectedStatus, expectedName := http.StatusOK, "new"
			switch scenario {
			case "settings-failure":
				expectedStatus, expectedName = http.StatusInternalServerError, "old"
			case "admins-failure":
				expectedStatus = http.StatusInternalServerError
			case "invalid-json", "null":
				expectedStatus, expectedName = http.StatusBadRequest, "old"
			}
			if response.Code != expectedStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if factorio.GetFactorioServer().Settings["preserved"] != true {
				t.Fatal("omitted settings field lost")
			}
			if factorio.GetFactorioServer().Settings["name"] != expectedName {
				t.Fatal("incorrect in-memory settings after save")
			}
			if scenario == "settings-failure" {
				factorio.GetFactorioServer().Savefile = "test.zip"
				start := httptest.NewRecorder()
				StartServer(start, httptest.NewRequest(http.MethodPost, "/api/server/start", strings.NewReader(`{}`)))
				if start.Code != http.StatusInternalServerError || !strings.Contains(start.Body.String(), config.SettingsFile) {
					t.Fatalf("startup failed to report settings error: %s", start.Body.String())
				}
			}
			if scenario == "settings-failure" && !strings.Contains(response.Body.String(), config.SettingsFile) {
				t.Fatal("failure omitted settings path")
			}
			if scenario == "admins-failure" && !strings.Contains(response.Body.String(), "Server settings saved, but") {
				t.Fatal("partial save not reported")
			}
			if expectedName == "new" {
				saved, err := os.ReadFile(config.SettingsFile)
				if err != nil {
					t.Fatal(err)
				}
				var decoded map[string]interface{}
				if err := json.Unmarshal(saved, &decoded); err != nil || decoded["name"] != "new" {
					t.Fatal("persisted settings mismatch")
				}
			}
		})
	}
}
