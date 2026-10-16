package api

import (
	"context"
	"github.com/OpenFactorioServerManager/factorio-server-manager/bootstrap"
	"github.com/OpenFactorioServerManager/factorio-server-manager/telemetry"
	"net/http"
	"time"
)

func UPSHistory(w http.ResponseWriter, r *http.Request) {
	config := bootstrap.GetConfig()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	history, err := telemetry.Fetch(ctx, &http.Client{Timeout: 5 * time.Second}, config.PrometheusURL, config.TelemetryLabels, time.Now())
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		WriteResponse(w, "UPS telemetry is temporarily unavailable.")
		return
	}
	WriteResponse(w, history)
}
