package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/config"
)

// handleComputersCommand manages Packet Computer records. Connections are
// established at process startup with --computer; slash commands never open a
// network connection implicitly.
func (a *App) handleComputersCommand(args []string) (tea.Model, tea.Cmd) {
	if a.deps.Config != nil && !a.deps.Config.PacketComputers.IsEnabled() {
		a.conversation.AppendSystem("computers: Packet Computers integration is disabled; enable [packet_computers].enabled or set PACKETCODE_PACKET_COMPUTERS_ENABLED=true")
		return a, nil
	}
	reg, err := loadComputerRegistry()
	if err != nil {
		a.conversation.AppendSystem(fmt.Sprintf("computers: %s", err))
		return a, nil
	}

	if len(args) == 0 {
		a.conversation.AppendSystem(renderComputersTable(reg.List()))
		return a, nil
	}

	switch args[0] {
	case "register":
		if len(args) != 3 {
			a.conversation.AppendSystem("computers: usage /computers register <name> <absolute-root>")
			return a, nil
		}
		root, absErr := filepath.Abs(args[2])
		if absErr != nil {
			a.conversation.AppendSystem("computers: " + absErr.Error())
			return a, nil
		}
		if _, exists := reg.Get(args[1]); exists {
			a.conversation.AppendSystem(fmt.Sprintf("computers: name %q is already registered", args[1]))
			return a, nil
		}
		created, createErr := reg.Upsert(computers.Computer{
			Name:         args[1],
			Kind:         computers.KindLocal,
			ProjectRoots: []string{filepath.Clean(root)},
			Capabilities: computers.Capabilities{Shell: true, Filesystem: true, Jobs: true},
		})
		if createErr != nil {
			a.conversation.AppendSystem("computers: " + createErr.Error())
			return a, nil
		}
		a.conversation.AppendSystem(fmt.Sprintf("registered local computer %s (%s)", created.Name, created.ProjectRoots[0]))
		return a, nil
	case "ssh":
		computer, parseErr := parseSSHComputerArgs(args[1:])
		if parseErr != nil {
			a.conversation.AppendSystem("computers: " + parseErr.Error())
			return a, nil
		}
		if _, exists := reg.Get(computer.Name); exists {
			a.conversation.AppendSystem(fmt.Sprintf("computers: name %q is already registered", computer.Name))
			return a, nil
		}
		created, createErr := reg.Upsert(computer)
		if createErr != nil {
			a.conversation.AppendSystem("computers: " + createErr.Error())
			return a, nil
		}
		a.conversation.AppendSystem(fmt.Sprintf(
			"registered SSH computer %s (%s@%s:%d, root %s); start with --computer %s",
			created.Name, created.SSHUser, created.SSHHost, created.SSHPort, created.ProjectRoots[0], created.Name,
		))
		return a, nil
	case "remove":
		if len(args) < 2 || len(args) > 3 || (len(args) == 3 && args[2] != "--yes") {
			a.conversation.AppendSystem("computers: usage /computers remove <name> --yes")
			return a, nil
		}
		if len(args) != 3 {
			a.conversation.AppendSystem(fmt.Sprintf("computers: refusing to remove without --yes; re-run: /computers remove %s --yes", args[1]))
			return a, nil
		}
		removed, removeErr := reg.Remove(args[1])
		if removeErr != nil {
			a.conversation.AppendSystem("computers: " + removeErr.Error())
			return a, nil
		}
		if !removed {
			a.conversation.AppendSystem(fmt.Sprintf("computers: no computer named %q", args[1]))
			return a, nil
		}
		a.conversation.AppendSystem(fmt.Sprintf("removed computer %s", args[1]))
		return a, nil
	case "status":
		if len(args) < 2 {
			a.conversation.AppendSystem("computers: usage /computers status <name>")
			return a, nil
		}
		c, ok := reg.Get(args[1])
		if !ok {
			a.conversation.AppendSystem(fmt.Sprintf("computers: no computer named %q", args[1]))
			return a, nil
		}
		a.conversation.AppendSystem(renderComputerDetail(c))
		return a, nil
	default:
		c, ok := reg.Get(args[0])
		if !ok {
			a.conversation.AppendSystem(fmt.Sprintf(
				"computers: no computer named %q (try /computers to list, /computers status <name> for detail)",
				args[0],
			))
			return a, nil
		}
		a.conversation.AppendSystem(renderComputerDetail(c))
		return a, nil
	}
}

func parseSSHComputerArgs(args []string) (computers.Computer, error) {
	const usage = "usage /computers ssh <name> <user@host> <absolute-root> --fingerprint <SHA256:...> [--port N] [--identity PATH]"
	if len(args) < 5 {
		return computers.Computer{}, errors.New(usage)
	}
	name, target, root := args[0], args[1], args[2]
	at := strings.LastIndex(target, "@")
	if at <= 0 || at == len(target)-1 {
		return computers.Computer{}, fmt.Errorf("SSH target must be user@host")
	}
	if !strings.HasPrefix(root, "/") {
		return computers.Computer{}, fmt.Errorf("SSH project root must be an absolute POSIX path")
	}
	computer := computers.Computer{
		Name:         name,
		Kind:         computers.KindSSH,
		SSHUser:      target[:at],
		SSHHost:      strings.Trim(target[at+1:], "[]"),
		SSHPort:      22,
		ProjectRoots: []string{root},
		Capabilities: computers.Capabilities{Shell: true, Filesystem: true, Jobs: true},
	}
	for i := 3; i < len(args); {
		switch args[i] {
		case "--fingerprint":
			if i+1 >= len(args) {
				return computers.Computer{}, fmt.Errorf("--fingerprint requires a value")
			}
			computer.SSHHostFingerprint = args[i+1]
			i += 2
		case "--port":
			if i+1 >= len(args) {
				return computers.Computer{}, fmt.Errorf("--port requires a value")
			}
			port, err := strconv.Atoi(args[i+1])
			if err != nil || port < 1 || port > 65535 {
				return computers.Computer{}, fmt.Errorf("--port must be between 1 and 65535")
			}
			computer.SSHPort = port
			i += 2
		case "--identity":
			if i+1 >= len(args) {
				return computers.Computer{}, fmt.Errorf("--identity requires a value")
			}
			computer.SSHIdentityFile = args[i+1]
			i += 2
		default:
			return computers.Computer{}, fmt.Errorf("unexpected argument %q; %s", args[i], usage)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(computer.SSHHostFingerprint), "SHA256:") {
		return computers.Computer{}, fmt.Errorf("--fingerprint must be an OpenSSH SHA256 fingerprint; PacketCode refuses unpinned SSH hosts")
	}
	return computer, nil
}

func loadComputerRegistry() (*computers.Registry, error) {
	dir, err := config.ComputersDir()
	if err != nil {
		return nil, err
	}
	return computers.Load(dir)
}

func renderComputersTable(list []computers.Computer) string {
	if len(list) == 0 {
		return "no computers registered\n" +
			"Register SSH: /computers ssh <name> <user@host> <absolute-root> " +
			"--fingerprint <SHA256:...> [--identity PATH]"
	}
	var b strings.Builder
	b.WriteString("NAME             KIND     STATUS    REACHABLE  ROOTS  CAPABILITIES\n")
	for _, c := range list {
		reach := "no"
		if c.Reachable() {
			reach = "yes"
		}
		fmt.Fprintf(&b, "%-16s %-8s %-9s %-10s %-6d %s\n",
			trunc(c.Name, 16),
			trunc(string(c.Kind), 8),
			trunc(string(c.Status), 9),
			reach,
			len(c.ProjectRoots),
			capabilitySummary(c.Capabilities),
		)
	}
	b.WriteString("\nstatus is the last stored value — direct SSH sessions do not provide a heartbeat. " +
		"Start one with: packetcode --computer <name>")
	return strings.TrimRight(b.String(), "\n")
}

func renderComputerDetail(c computers.Computer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", c.Name, c.Kind)
	fmt.Fprintf(&b, "  id           %s\n", c.ID)
	fmt.Fprintf(&b, "  status       %s", c.Status)
	if c.DaemonVersion == "" {
		b.WriteString("  (never contacted — no daemon has reported in)")
	}
	b.WriteString("\n")
	if c.Kind == computers.KindSSH {
		target := c.SSHHost
		if c.SSHUser != "" {
			target = c.SSHUser + "@" + target
		}
		if c.SSHPort > 0 {
			target = fmt.Sprintf("%s:%d", target, c.SSHPort)
		}
		fmt.Fprintf(&b, "  ssh          %s\n", target)
		fingerprint := c.SSHHostFingerprint
		if fingerprint == "" {
			fingerprint = "not pinned (connection disabled)"
		}
		fmt.Fprintf(&b, "  host key     %s\n", fingerprint)
		if c.SSHIdentityFile != "" {
			fmt.Fprintf(&b, "  identity     %s\n", c.SSHIdentityFile)
		}
	}
	if c.OS != "" || c.Arch != "" {
		fmt.Fprintf(&b, "  platform     %s/%s\n", c.OS, c.Arch)
	}
	if len(c.ProjectRoots) > 0 {
		fmt.Fprintf(&b, "  roots        %s\n", strings.Join(c.ProjectRoots, ", "))
	}
	fmt.Fprintf(&b, "  capabilities %s\n", capabilitySummary(c.Capabilities))
	fmt.Fprintf(&b, "  policy       network=%s write=%s shell=%s secrets=%s approval=%s\n",
		c.Policy.Network, c.Policy.Write, c.Policy.Shell, c.Policy.Secrets, c.Policy.Approval)
	if !c.LastSeen.IsZero() {
		fmt.Fprintf(&b, "  last seen    %s\n", c.LastSeen.Format("2006-01-02 15:04:05 MST"))
	}
	if c.Kind == computers.KindSSH {
		fmt.Fprintf(&b, "\nconnect with: packetcode --computer %s", c.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

func capabilitySummary(c computers.Capabilities) string {
	var on []string
	if c.Shell {
		on = append(on, "shell")
	}
	if c.Filesystem {
		on = append(on, "fs")
	}
	if c.Jobs {
		on = append(on, "jobs")
	}
	if c.Terminals {
		on = append(on, "terminals")
	}
	if c.Browser {
		on = append(on, "browser")
	}
	if len(on) == 0 {
		return "none declared"
	}
	return strings.Join(on, ",")
}
