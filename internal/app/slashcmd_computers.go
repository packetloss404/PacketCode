package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/config"
)

// handleComputersCommand implements the read-only /computers surface for
// Packet Computers Milestone A.
//
// It lists and inspects registry records only. There is no daemon, no
// transport, and no spawn-on-computer yet, so this command never contacts a
// machine — every status it prints is a stored record, and it says so rather
// than implying a live probe.
func (a *App) handleComputersCommand(args []string) (tea.Model, tea.Cmd) {
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
			"Packet Computers is registry-only for now: records live in " +
			"~/.packetcode/computers/registry.json and nothing connects to them yet."
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
	b.WriteString("\nstatus is the last stored value — no heartbeat exists yet, so " +
		"'unknown' means never contacted, not offline.")
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
	b.WriteString("\nregistry-only: packetcode cannot yet run work on this computer.")
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
