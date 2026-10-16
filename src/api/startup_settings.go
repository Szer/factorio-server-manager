package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/modsettings"
)

func startupResponse(w http.ResponseWriter, document *modsettings.Document, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, modsettings.ErrConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
			err = errors.New("No mod-settings.dat exists yet. Upload one from your Factorio client first.")
		}
		w.WriteHeader(status)
		WriteResponse(w, err.Error())
		return
	}
	WriteResponse(w, document)
}
func GetStartupSettings(w http.ResponseWriter, r *http.Request) {
	d, e := modsettings.Read(filepath.Join(bootstrap.GetConfig().FactorioModsDir, "mod-settings.dat"))
	startupResponse(w, d, e)
}
func SaveStartupSettings(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Revision string                        `json:"revision"`
		Changes  map[string]modsettings.Change `json:"changes"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&request); e != nil {
		startupResponse(w, nil, errors.New("Invalid settings request"))
		return
	}
	if e := decoder.Decode(new(interface{})); e != io.EOF {
		startupResponse(w, nil, errors.New("Unexpected trailing request data"))
		return
	}
	d, e := modsettings.Save(filepath.Join(bootstrap.GetConfig().FactorioModsDir, "mod-settings.dat"), request.Revision, request.Changes)
	startupResponse(w, d, e)
}
