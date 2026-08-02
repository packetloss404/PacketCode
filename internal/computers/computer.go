// Package computers holds Packet Computer records and runtime backends.
package computers

import (
	"fmt"
	"strings"
	"time"
)

// Kind discriminates how a computer is reached.
type Kind string

const (
	// KindLocal is the machine packetcode itself is running on.
	KindLocal Kind = "local"
	// KindSSH is a user-owned remote machine reached over SSH.
	KindSSH Kind = "ssh"
	// KindManaged is a future Packet-provisioned cloud machine. Records may
	// carry this kind, but Milestone A provisions nothing.
	KindManaged Kind = "managed"
)

// ValidKind reports whether k is a known kind.
func ValidKind(k Kind) bool {
	switch k {
	case KindLocal, KindSSH, KindManaged:
		return true
	}
	return false
}

// Status is the last known reachability of a computer.
//
// Milestone A has no heartbeat, so a stored status is a *record*, not a live
// probe. Registry reads therefore report StatusUnknown for anything that has
// never been contacted rather than implying freshness the daemon would have
// to provide.
type Status string

const (
	StatusUnknown Status = "unknown"
	StatusOnline  Status = "online"
	StatusOffline Status = "offline"
)

// Capabilities records what a computer is declared to support. These are
// user-maintained assertions in Milestone A; the daemon will replace them
// with probed values.
type Capabilities struct {
	Shell      bool `json:"shell"`
	Filesystem bool `json:"filesystem"`
	Jobs       bool `json:"jobs"`
	Terminals  bool `json:"terminals"`
	Browser    bool `json:"browser"`
}

// PolicyMode is the shared approval vocabulary from PACKETCOMPUTERS.md's
// policy model. Conservative defaults are the point: a computer that has
// not been configured must not read as permissive.
type PolicyMode string

const (
	PolicyDeny  PolicyMode = "deny"
	PolicyAsk   PolicyMode = "ask"
	PolicyAllow PolicyMode = "allow"
)

// ValidPolicyMode reports whether m is a known policy mode.
func ValidPolicyMode(m PolicyMode) bool {
	switch m {
	case PolicyDeny, PolicyAsk, PolicyAllow:
		return true
	}
	return false
}

// ApprovalMode is the approval axis from PACKETCOMPUTERS.md's shared policy
// model.
//
// This is deliberately a string enum rather than a bool. The safe default is
// "every action is approved explicitly", and a bool whose safe value is true
// cannot survive a JSON round-trip — an absent field would decode to false
// and silently widen trust.
type ApprovalMode string

const (
	ApprovalExplicit       ApprovalMode = "explicit"
	ApprovalTrustWorkspace ApprovalMode = "trust-workspace"
	ApprovalTrustComputer  ApprovalMode = "trust-computer"
)

// ValidApprovalMode reports whether m is a known approval mode.
func ValidApprovalMode(m ApprovalMode) bool {
	switch m {
	case ApprovalExplicit, ApprovalTrustWorkspace, ApprovalTrustComputer:
		return true
	}
	return false
}

// Policy is the per-computer trust configuration. Defaults are "ask" for
// everything a remote machine could do, and "deny" for secrets: a remote
// computer must not silently inherit local credentials.
type Policy struct {
	Network  PolicyMode   `json:"network"`
	Write    PolicyMode   `json:"write"`
	Shell    PolicyMode   `json:"shell"`
	Secrets  PolicyMode   `json:"secrets"`
	Approval ApprovalMode `json:"approval"`
}

// DefaultPolicy returns the conservative policy applied to records that do
// not specify one.
func DefaultPolicy() Policy {
	return Policy{
		Network:  PolicyAsk,
		Write:    PolicyAsk,
		Shell:    PolicyAsk,
		Secrets:  PolicyDeny,
		Approval: ApprovalExplicit,
	}
}

// normalize fills unset or invalid policy fields with the conservative
// default rather than trusting whatever was on disk.
func (p Policy) normalize() Policy {
	def := DefaultPolicy()
	if !ValidPolicyMode(p.Network) {
		p.Network = def.Network
	}
	if !ValidPolicyMode(p.Write) {
		p.Write = def.Write
	}
	if !ValidPolicyMode(p.Shell) {
		p.Shell = def.Shell
	}
	if !ValidPolicyMode(p.Secrets) {
		p.Secrets = def.Secrets
	}
	if !ValidApprovalMode(p.Approval) {
		p.Approval = def.Approval
	}
	return p
}

// Computer is one durable machine record.
//
// The zero value is not usable; construct records through the registry so
// validation and policy normalization always run.
type Computer struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         Kind         `json:"kind"`
	Status       Status       `json:"status,omitempty"`
	LastSeen     time.Time    `json:"last_seen,omitempty"`
	OS           string       `json:"os,omitempty"`
	Arch         string       `json:"arch,omitempty"`
	ProjectRoots []string     `json:"project_roots,omitempty"`
	Capabilities Capabilities `json:"capabilities"`
	Policy       Policy       `json:"policy"`

	// SSHHost/SSHPort/SSHUser describe how to reach a KindSSH computer.
	// Passwords and private-key contents are deliberately absent: this file is
	// not a place for secrets.
	SSHHost string `json:"ssh_host,omitempty"`
	SSHPort int    `json:"ssh_port,omitempty"`
	SSHUser string `json:"ssh_user,omitempty"`

	// SSHIdentityFile is an optional path to a private key on the local
	// computer. It is configuration, not key material; private keys are never
	// copied into the registry. When empty, the SSH backend tries SSH_AUTH_SOCK
	// and the conventional ~/.ssh identity files.
	SSHIdentityFile string `json:"ssh_identity_file,omitempty"`
	// SSHHostFingerprint is the exact SHA256 host-key fingerprint approved by
	// the user. SSH connections fail closed when it is absent or changes.
	SSHHostFingerprint string `json:"ssh_host_fingerprint,omitempty"`

	// DaemonVersion is recorded once a daemon reports in. It stays empty
	// through Milestone A and is a reliable "never contacted" signal.
	DaemonVersion string `json:"daemon_version,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// ErrInvalid is returned for records that cannot be stored as given.
type ErrInvalid struct{ Reason string }

func (e *ErrInvalid) Error() string { return "invalid computer: " + e.Reason }

// validName restricts names to characters that are safe as a filename and
// unambiguous to type at the /computers prompt.
func validName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &ErrInvalid{Reason: "name is empty"}
	}
	if len(name) > 64 {
		return &ErrInvalid{Reason: fmt.Sprintf("name %q exceeds 64 characters", name)}
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return &ErrInvalid{Reason: fmt.Sprintf("name %q: use only letters, digits, '_' or '-'", name)}
		}
	}
	return nil
}

// normalize validates and fills a record, returning the storable form.
func (c Computer) normalize(now time.Time) (Computer, error) {
	if err := validName(c.Name); err != nil {
		return Computer{}, err
	}
	if !ValidKind(c.Kind) {
		return Computer{}, &ErrInvalid{Reason: fmt.Sprintf("unknown kind %q", c.Kind)}
	}
	if c.Kind == KindSSH && strings.TrimSpace(c.SSHHost) == "" {
		return Computer{}, &ErrInvalid{Reason: "ssh computer requires ssh_host"}
	}
	if c.Kind == KindSSH && (c.SSHPort < 0 || c.SSHPort > 65535) {
		return Computer{}, &ErrInvalid{Reason: fmt.Sprintf("ssh_port %d out of range", c.SSHPort)}
	}
	if c.Kind == KindSSH && c.SSHPort == 0 {
		c.SSHPort = 22
	}
	if c.ID == "" {
		c.ID = "pc_" + strings.ToLower(c.Name)
	}
	// Nothing has probed this machine, so never persist a live-looking
	// status that no heartbeat backs.
	if c.DaemonVersion == "" {
		c.Status = StatusUnknown
	}
	if c.Status == "" {
		c.Status = StatusUnknown
	}
	c.Policy = c.Policy.normalize()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	return c, nil
}

// Reachable reports whether the record carries enough information to be
// contacted once a transport exists. It performs no I/O.
func (c Computer) Reachable() bool {
	switch c.Kind {
	case KindLocal:
		return true
	case KindSSH:
		return strings.TrimSpace(c.SSHHost) != "" &&
			strings.TrimSpace(c.SSHUser) != "" &&
			len(c.ProjectRoots) > 0 &&
			strings.TrimSpace(c.SSHHostFingerprint) != ""
	default:
		return false
	}
}
