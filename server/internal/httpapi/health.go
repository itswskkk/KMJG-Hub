package httpapi

import "net/http"

// serverVersion identifies this build for the Desktop Client's Connect
// Server reachability check (docs/UX.md "First Launch": "After a successful
// connection, the user continues to authentication"). Full Client/Server
// compatibility negotiation is deferred, per
// docs/ARCHITECTURE.md "Client and Server Compatibility" ("exact supported
// compatibility policy will be defined when KMJG Hub begins producing
// versioned releases").
const serverVersion = "0.1.0"

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Service: "kmjg-hub-server",
		Version: serverVersion,
	})
}
