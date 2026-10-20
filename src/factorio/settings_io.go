package factorio

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/OpenFactorioServerManager/factorio-server-manager/logging"
)

// SettingsMu serializes settings snapshots and writes from HTTP requests and startup.
var SettingsMu sync.Mutex

func DebugSettingsFile(operation, path string) {
	if !logging.DebugEnabled() {
		return
	}
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	logging.Debugf("settings %s path=%q process_euid=%d process_egid=%d", operation, path, os.Geteuid(), os.Getegid())
	for _, target := range []string{path, filepath.Dir(path)} {
		info, err := os.Stat(target)
		if err != nil {
			logging.Debugf("settings metadata path=%q error=%v", target, err)
			continue
		}
		logging.Debugf("settings metadata path=%q mode=%s size=%d %s", target, info.Mode(), info.Size(), settingsOwner(info))
	}
}

func WriteSettingsFile(path string, data []byte, mode os.FileMode, source string) error {
	DebugSettingsFile("write source="+source, path)
	if err := os.WriteFile(path, data, mode); err != nil {
		err = fmt.Errorf("settings write failed (%s) path=%q: %w", source, path, err)
		log.Print(err)
		return err
	}
	logging.Debugf("settings write succeeded source=%s path=%q bytes=%d", source, path, len(data))
	return nil
}

func (server *Server) writeSettingsForStart(path string) error {
	SettingsMu.Lock()
	defer SettingsMu.Unlock()
	data, err := json.MarshalIndent(server.Settings, "", "  ")
	if err != nil {
		err = fmt.Errorf("cannot encode server settings for startup: %w", err)
		log.Print(err)
		return err
	}
	return WriteSettingsFile(path, data, 0644, "game-startup-memory-snapshot")
}
