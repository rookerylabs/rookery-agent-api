// Package api is the wire contract between the Rookery control plane and a
// rookery-agent. It is deliberately types-only: no HTTP, no logic, so both
// repos can import it without dragging in dependencies. One source of truth
// for the JSON, so the two sides can never drift.
//
// One agent runs per HOST (rootful) and serves EVERY scope on that host: the
// system (rootful) manager plus each user's rootless session. The control
// plane discovers the scopes via GET /v1/scopes, then selects one per request
// with ?scope=<id>. So "one agent = one node", covering all of a host's
// rootful and rootless containers.
package api

// Version is the wire-contract version, surfaced in HostInfo so a mismatched
// control plane and agent are diagnosable. Agent and control plane are built
// and released together, so this is a diagnostic, not a compatibility gate.
const Version = "1"

// ScopeParam is the query-string key selecting which of a host's scopes a
// request targets, e.g. /v1/units?scope=tobagin. Its value is a Scope.ID.
const ScopeParam = "scope"

// SystemScopeID is the Scope.ID of the rootful system manager. User scopes use
// the username as their ID.
const SystemScopeID = "system"

// Scope is one systemd manager + podman store on the agent's host: the system
// manager, or a single user's rootless session.
type Scope struct {
	ID                string `json:"id"`     // "system" or the username
	Label             string `json:"label"`  // display name; "system" or the username
	User              string `json:"user"`   // "" for the system scope
	System            bool   `json:"system"` // true = rootful system manager
	PodmanVersion     string `json:"podmanVersion"`
	ContainersRunning int    `json:"containersRunning"`
	ContainersTotal   int    `json:"containersTotal"`
	// Error is set when the agent found the scope but could not query its
	// podman (socket down, lingering off); the scope still lists so the node
	// shows it, degraded rather than missing.
	Error string `json:"error,omitempty"`
}

// HostInfo is GET /v1/scopes: the agent's host identity plus every scope it
// manages. The control plane turns each Scope into a node area.
type HostInfo struct {
	Host         string  `json:"host"`
	AgentVersion string  `json:"agentVersion"`
	WireVersion  string  `json:"wireVersion"`
	Scopes       []Scope `json:"scopes"`
}

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

// Unit is one Quadlet unit in a scope, with its live systemd status.
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

// Resource is one live podman network or volume in a scope, tagged whether a
// Quadlet unit in that scope owns it (the agent computes managed itself, since
// it has both the podman store and the unit files).
type Resource struct {
	Kind    string `json:"kind"` // "network" | "volume"
	Name    string `json:"name"`
	Driver  string `json:"driver,omitempty"`
	Detail  string `json:"detail,omitempty"` // subnet for networks, mountpoint for volumes
	Managed bool   `json:"managed"`
	Used    bool   `json:"used"` // referenced by a container in the scope
}

// HostMetrics is GET /v1/metrics: a point-in-time snapshot of the agent host's
// health, so an agent-backed node shows the same CPU/mem/load strip as a local
// or ssh node. Field tags match the control plane's hostinfo.Metrics exactly so
// it decodes straight through. CPUPct is -1 until the agent has two samples.
type HostMetrics struct {
	Hostname      string  `json:"hostname"`
	Kernel        string  `json:"kernel"`
	Load1         float64 `json:"load1"`
	Cores         int     `json:"cores"`
	CPUPct        int     `json:"cpuPct"`
	MemTotalKB    int64   `json:"memTotalKb"`
	MemAvailKB    int64   `json:"memAvailKb"`
	UptimeSeconds int64   `json:"uptimeSeconds"`
}

// GPUDevice is one entry of GET /v1/gpus: a GPU on the agent's host. Tags match
// the control plane's gpu.Device (minus Host, which the control plane fills in
// with the node label). Unknown metrics are -1 so the UI tells "0%" from "can't
// tell".
type GPUDevice struct {
	Index          int    `json:"index"`
	Vendor         string `json:"vendor"` // nvidia, amd, intel
	Name           string `json:"name"`
	MemoryTotalMB  int    `json:"memoryTotalMb"`
	MemoryUsedMB   int    `json:"memoryUsedMb"`
	UtilizationPct int    `json:"utilizationPct"`
}

// ActionResult is returned by a lifecycle action (start/stop/...).
type ActionResult struct {
	Unit   string `json:"unit"`
	Action string `json:"action"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

// Lifecycle actions accepted at POST /v1/units/{name}/{action}?scope=<id>.
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

// HTTP endpoints. All are versioned under /v1; every request but health must
// carry Authorization: Bearer <token>. Every per-scope endpoint takes
// ?scope=<Scope.ID>.
const (
	PathHealth       = "/v1/healthz"       // GET  — no auth, liveness only
	PathScopes       = "/v1/scopes"        // GET  — HostInfo (host + all scopes)
	PathUnits        = "/v1/units"         // GET  — []Unit           ?scope=
	PathContainers   = "/v1/containers"    // GET  — []Container      ?scope=
	PathStats        = "/v1/stats"         // GET  — []Stat           ?scope=
	PathResources    = "/v1/resources"     // GET  — []Resource       ?scope=
	PathMetrics      = "/v1/metrics"       // GET  — HostMetrics (host-level, no scope)
	PathGPUs         = "/v1/gpus"          // GET  — []GPUDevice  (host-level, no scope)
	PathDaemonReload = "/v1/daemon-reload" // POST — reload scope's units ?scope=
	// Lifecycle: POST /v1/units/{name}/{action}?scope=<id> — ActionResult.
	PathUnitsPrefix = "/v1/units/"
	// Per-unit sub-resources under /v1/units/{name}/… (all ?scope=<id>):
	//   GET  …/file  — raw Quadlet file contents (text/plain)
	//   PUT  …/file  — write contents, then daemon-reload
	//   DELETE …/file — remove file, then daemon-reload
	//   GET  …/logs  — journal for the unit (text/plain); ?lines=N&since=…
	SubFile = "/file"
	SubLogs = "/logs"
)

// UnitFileURL / UnitLogsURL build the per-unit sub-resource paths so both
// sides derive them the same way. name is the Quadlet file name; callers
// append ?scope=<id>.
func UnitFileURL(name string) string { return PathUnitsPrefix + name + SubFile }
func UnitLogsURL(name string) string { return PathUnitsPrefix + name + SubLogs }

// HeaderAuth is the bearer-token header the agent requires on every
// non-health request.
const HeaderAuth = "Authorization"
