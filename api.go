// Package api is the wire contract between the Rookery control plane and a
// rookery-agent. It is deliberately types-only: no HTTP, no logic, so both
// repos can import it without dragging in dependencies. One source of truth
// for the JSON, so the two sides can never drift.
//
// An agent speaks for exactly ONE scope (its own systemd session + podman
// socket). The control plane knows which agent maps to which scope, so the
// requests below carry no scope selector — the agent IS the scope.
package api

// Version is the wire-contract version, surfaced in Info so a mismatched
// control plane and agent are diagnosable. Agent and control plane are built
// and released together, so this is a diagnostic, not a compatibility gate.
const Version = "1"

// Status mirrors the subset of `systemctl show` state Rookery surfaces. Field
// names and JSON tags match the control plane's systemd.UnitStatus exactly so
// the connector can decode straight into its existing type.
type Status struct {
	Load     string `json:"load"`     // loaded, not-found, ...
	Active   string `json:"active"`   // active, inactive, failed, activating, ...
	Sub      string `json:"sub"`      // running, exited, dead, auto-restart, ...
	UnitFile string `json:"unitFile"` // enabled, disabled, generated, ...
	Result   string `json:"result"`   // success, exit-code, signal, ...
	ExitCode int    `json:"exitCode"` // ExecMainStatus of the last run
	Restarts int    `json:"restarts"` // NRestarts — a climbing value flags a restart loop
}

// Unit is one Quadlet unit in this scope, with its live systemd status.
type Unit struct {
	Name    string `json:"name"`    // file name, e.g. "ntfy.container"
	Kind    string `json:"kind"`    // container|pod|network|volume|kube|image|build
	Path    string `json:"path"`    // absolute path of the quadlet file
	Service string `json:"service"` // generated unit, e.g. "ntfy.service"
	Status  Status `json:"status"`
}

// Container is the `podman ps --all` subset the dashboard needs. Health is the
// container's healthcheck status ("healthy"/"unhealthy"/""), resolved by the
// agent locally so the control plane needs no extra round trip.
type Container struct {
	ID      string            `json:"id"`
	Names   []string          `json:"names"`
	Image   string            `json:"image"`
	State   string            `json:"state"`
	IsInfra bool              `json:"isInfra"`
	Labels  map[string]string `json:"labels"`
	Health  string            `json:"health,omitempty"`
}

// Stat is one live resource sample for a running container.
type Stat struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	CPUPct   float64 `json:"cpuPct"`
	MemBytes int64   `json:"memBytes"`
}

// Info identifies the scope this agent serves and its podman backend, so the
// control plane can label the node and show host/version detail.
type Info struct {
	Scope             string `json:"scope"`   // human label, e.g. "user:tobagin"
	User              string `json:"user"`    // "" for the system (rootful) scope
	System            bool   `json:"system"`  // true = rootful system manager
	Host              string `json:"host"`    // hostname the agent runs on
	PodmanVersion     string `json:"podmanVersion"`
	ContainersRunning int    `json:"containersRunning"`
	ContainersTotal   int    `json:"containersTotal"`
	AgentVersion      string `json:"agentVersion"`
	WireVersion       string `json:"wireVersion"` // == Version above
}

// ActionResult is returned by a lifecycle action (start/stop/...).
type ActionResult struct {
	Unit   string `json:"unit"`
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// Lifecycle actions accepted at POST {PathUnits}/{name}/{action}.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
	ActionEnable  = "enable"
	ActionDisable = "disable"
)

// ValidAction reports whether a is a lifecycle action the agent honors. The
// agent and the control plane both call this so the allow-list lives in one
// place.
func ValidAction(a string) bool {
	switch a {
	case ActionStart, ActionStop, ActionRestart, ActionEnable, ActionDisable:
		return true
	}
	return false
}

// HTTP endpoints. All are versioned under /v1; every request must carry
// Authorization: Bearer <token>.
const (
	PathHealth       = "/v1/healthz"       // GET  — no auth, liveness only
	PathInfo         = "/v1/info"          // GET  — Info
	PathUnits        = "/v1/units"         // GET  — []Unit
	PathContainers   = "/v1/containers"    // GET  — []Container
	PathStats        = "/v1/stats"         // GET  — []Stat
	PathDaemonReload = "/v1/daemon-reload" // POST — reload this scope's units
	// Lifecycle: POST /v1/units/{name}/{action} — ActionResult.
	PathUnitsPrefix = "/v1/units/"
	// Per-unit sub-resources under /v1/units/{name}/…:
	//   GET  …/file  — raw Quadlet file contents (text/plain)
	//   PUT  …/file  — write contents, then daemon-reload
	//   GET  …/logs  — journal for the unit (text/plain); ?lines=N&since=…
	SubFile = "/file"
	SubLogs = "/logs"
)

// UnitFileURL / UnitLogsURL build the per-unit sub-resource paths so both
// sides derive them the same way. name is the Quadlet file name.
func UnitFileURL(name string) string { return PathUnitsPrefix + name + SubFile }
func UnitLogsURL(name string) string { return PathUnitsPrefix + name + SubLogs }

// HeaderAuth is the bearer-token header the agent requires on every
// non-health request.
const HeaderAuth = "Authorization"
