package domain

import "time"

type AuthMode string

const (
	AuthModeEnvActive     AuthMode = "env-active"
	AuthModeProfileActive AuthMode = "profile-active"
)

type AuthContext struct {
	Mode    AuthMode `json:"mode"`
	Profile string   `json:"profile,omitempty"`
	Region  string   `json:"region"`
}

type Identity struct {
	Account string `json:"account"`
	ARN     string `json:"arn"`
	UserID  string `json:"userId,omitempty"`
}

type SSMStatus string

const (
	SSMStatusUnknown        SSMStatus = "unknown"
	SSMStatusNotManaged     SSMStatus = "not-managed"
	SSMStatusOnline         SSMStatus = "online"
	SSMStatusConnectionLost SSMStatus = "connection-lost"
	SSMStatusError          SSMStatus = "error"
)

type AgentInfo struct {
	Version          string `json:"version,omitempty"`
	LastPingUnixTime int64  `json:"lastPingUnixTime,omitempty"`
	PlatformType     string `json:"platformType,omitempty"`
}

type SecurityGroup struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type Instance struct {
	ID             string            `json:"id"`
	Name           string            `json:"name,omitempty"`
	State          string            `json:"state"`
	Type           string            `json:"type,omitempty"`
	AMIID          string            `json:"amiId,omitempty"`
	PrivateIP      string            `json:"privateIp,omitempty"`
	PublicIP       string            `json:"publicIp,omitempty"`
	Region         string            `json:"region"`
	AZ             string            `json:"availabilityZone,omitempty"`
	SSMStatus      SSMStatus         `json:"ssmStatus"`
	Agent          AgentInfo         `json:"agent,omitempty"`
	CreatedAt      int64             `json:"createdAt,omitempty"`
	SecurityGroups []SecurityGroup   `json:"securityGroups,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ListResult struct {
	Auth      AuthContext `json:"auth"`
	Region    string      `json:"region"`
	Account   string      `json:"account"`
	ARN       string      `json:"arn"`
	Warnings  []Warning   `json:"warnings"`
	Instances []Instance  `json:"instances"`
}

type TunnelState string

const (
	TunnelStateStarting TunnelState = "starting"
	TunnelStateRunning  TunnelState = "running"
	TunnelStateStopped  TunnelState = "stopped"
	TunnelStateFailed   TunnelState = "failed"
)

type Tunnel struct {
	ID         string
	InstanceID string
	Region     string
	Profile    string
	LocalPort  int
	RemotePort int
	Host       string
	StartedAt  time.Time
	State      TunnelState
	Err        error
	Output     string
}
