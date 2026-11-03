package httpapi

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/agentsvc"
)

// prober is the shared worker-health prober for the agent routes. One instance
// so the short-lived cache is actually shared across requests.
var prober = agentsvc.New()

// agentHealthHandler serves nabu's cached view of the home worker's health to any
// signed-in student. It drives the serverstatus widget, the server-offline
// banner, and the teacher dashboard header. It always 200s with a Health body:
// an unreachable worker is Alive:false, not an HTTP error.
func agentHealthHandler(e *core.RequestEvent) error {
	h := prober.Health(e.Request.Context())
	return e.JSON(http.StatusOK, h)
}

// requireAuth gates a route to any authenticated principal (student, staff, or
// superuser). Health is safe for every signed-in user to read.
func requireAuth(e *core.RequestEvent) error {
	if e.Auth != nil || e.HasSuperuserAuth() {
		return e.Next()
	}
	return e.UnauthorizedError("authentication required", nil)
}
