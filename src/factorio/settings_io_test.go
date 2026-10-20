package factorio

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenFactorioServerManager/factorio-server-manager/logging"
)

func TestSettingsDiagnosticsAndWriteFailures(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(old); logging.Configure("") })
	path := filepath.Join(t.TempDir(), "server-settings.json")
	secret := "test-secret-do-not-log"
	for _, level := range []string{"info", "debug"} {
		logging.Configure(level)
		output.Reset()
		if err := WriteSettingsFile(path, []byte(secret), 0600, "test"); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.String(), secret) {
			t.Fatal("settings content leaked")
		}
		if strings.Contains(output.String(), "process_euid=") != (level == "debug") {
			t.Fatal("incorrect diagnostic level")
		}
		output.Reset()
		if err := WriteSettingsFile(filepath.Dir(path), []byte(secret), 0600, "test"); err == nil {
			t.Fatal("directory write succeeded")
		}
		if !strings.Contains(output.String(), "settings write failed") {
			t.Fatal("write failure not logged")
		}
	}
}

func TestStartupSettingsWriteFailure(t *testing.T) {
	server := Server{Settings: map[string]interface{}{"name": "new"}}
	if err := server.writeSettingsForStart(t.TempDir()); err == nil {
		t.Fatal("startup accepted failed write")
	}
	path := filepath.Join(t.TempDir(), "server-settings.json")
	if err := server.writeSettingsForStart(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "new") {
		t.Fatalf("settings not written: %v", err)
	}
}

func TestSettingsPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write read-only files")
	}
	path := filepath.Join(t.TempDir(), "server-settings.json")
	if err := os.WriteFile(path, []byte("original"), 0400); err != nil {
		t.Fatal(err)
	}
	err := WriteSettingsFile(path, []byte("replacement"), 0600, "test")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected permission denial, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatal("failed write changed settings")
	}
}
