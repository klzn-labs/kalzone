package agentapi

const (
	NameHeader    = "X-Agent-Name"
	VersionHeader = "X-Agent-Version"
	ClaimPath     = "/agent/v1/claim"
	DesiredPath   = "/agent/v1/desired"
	DriftPath     = "/agent/v1/drift"
)

func EventsPath(deploymentID string) string {
	return "/agent/v1/deployments/" + deploymentID + "/events"
}

func FinishPath(deploymentID string) string {
	return "/agent/v1/deployments/" + deploymentID + "/finish"
}

type Spec struct {
	Port       int      `json:"port"`
	HealthPath string   `json:"health_path"`
	Host       string   `json:"host"`
	Aliases    []string `json:"aliases,omitempty"`
	Internal   bool     `json:"internal,omitempty"`
	MemoryMB   int      `json:"memory_mb"`
	CPUMillis  int      `json:"cpu_millis"`
	Replicas   int      `json:"replicas"`
	Command    string   `json:"command"`
}

type Workload struct {
	DeploymentID string            `json:"deployment_id"`
	Number       int               `json:"number"`
	Name         string            `json:"name"`
	Service      string            `json:"service"`
	Environment  string            `json:"environment"`
	ImageRef     string            `json:"image_ref"`
	Version      string            `json:"version"`
	Strategy     string            `json:"strategy"`
	Spec         Spec              `json:"spec"`
	Env          map[string]string `json:"env"`
	Secrets      map[string]string `json:"secrets"`
}

type Events struct {
	Lines []string `json:"lines"`
}

type Finish struct {
	Succeeded bool   `json:"succeeded"`
	Message   string `json:"message"`
}

type Desired struct {
	Name         string `json:"name"`
	Service      string `json:"service"`
	Environment  string `json:"environment"`
	ImageRef     string `json:"image_ref"`
	DeploymentID string `json:"deployment_id"`
}

type DesiredList struct {
	Workloads []Desired `json:"workloads"`
}

type Drift struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Observed    string `json:"observed"`
}
