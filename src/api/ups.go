package api

import (
	"encoding/json"
	"fmt"
	"github.com/OpenFactorioServerManager/factorio-server-manager/telemetry"
	"net/http"
	"time"
)

var UPS *telemetry.Service

func UPSHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if UPS == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		WriteResponse(w, "UPS telemetry is not initialized.")
		return
	}
	history, err := UPS.History(r.Context(), time.Now())
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		WriteResponse(w, "UPS telemetry is temporarily unavailable.")
		return
	}
	WriteResponse(w, history)
}

// Only public chart timings are exposed; backend configuration stays server-side.
func UPSClientSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/javascript")
	if UPS == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	data, _ := json.Marshal(UPS.ClientSettings())
	fmt.Fprintf(w, "window.fsmUPS = %s;", data)
}
