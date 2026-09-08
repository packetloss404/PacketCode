package permissions

import "testing"

// Policies also run against SSH backends, so both command languages must be
// handled regardless of which platform executes this test.
func TestPolicy_DenyPrefixCoversWindowsAliases(t *testing.T) {
	p := denyPushPolicy().WithRule("execute_command", DecisionAllow)
	for _, command := range []string{
		"GIT push origin main",
		"git.exe push origin main",
		"Git.EXE push origin main",
		"git PUSH origin main",
	} {
		t.Run(command, func(t *testing.T) {
			assertCommandDecision(t, p, command, DecisionDeny)
		})
	}
}

func TestPolicy_DenyPrefixEscalatesShellExpansionAndIndirection(t *testing.T) {
	p := denyPushPolicy().WithRule("execute_command", DecisionAllow)
	for _, command := range []string{
		"gi^t push origin main",
		"git %VERB% origin main",
		"echo %PAYLOAD%",
		"echo !PAYLOAD!",
		"git p* origin main",
		"g?t push origin main",
		"git p[us][sh]h origin main",
		"PoWeRsHeLl.EXE -Command Write-Output harmless",
		"CMD.EXE /c echo harmless",
		"call git push origin main",
		"start git push origin main",
		"if exist marker git push origin main",
		"if true; then git push origin main; fi",
		"release.cmd",
		"release.BAT",
	} {
		t.Run(command, func(t *testing.T) {
			assertCommandDecision(t, p, command, DecisionAsk)
		})
	}
}

func TestPolicy_ScopedAllowRefusesCMDExpansion(t *testing.T) {
	p := DefaultPolicy().WithCommandPrefixRule([]string{"echo"}, DecisionAllow)
	for _, command := range []string{
		"echo %PAYLOAD%",
		"echo !PAYLOAD!",
		"echo escaped^word",
	} {
		t.Run(command, func(t *testing.T) {
			assertCommandDecision(t, p, command, DecisionAsk)
		})
	}
	assertCommandDecision(t, p, "echo ordinary arguments", DecisionAllow)
	assertCommandDecision(t, p, "ECHO ordinary arguments", DecisionAsk)
	// Deliberately remembered complete programs retain byte-for-byte rules.
	assertCommandDecision(t, p.WithCommandRule("echo %PAYLOAD%", DecisionAllow), "echo %PAYLOAD%", DecisionAllow)
}

func TestPolicy_DenyPrefixRetainsExplicitOptionTokens(t *testing.T) {
	p := DefaultPolicy().WithProfile(ProfileAuto).
		WithCommandPrefixRule([]string{"rm", "-rf"}, DecisionDeny).
		WithRule("execute_command", DecisionAllow)
	for _, command := range []string{
		"rm -rf target",
		"rm -rf target && echo done",
		"echo start; rm -rf target",
		`rm "-rf" target`,
	} {
		t.Run(command, func(t *testing.T) {
			assertCommandDecision(t, p, command, DecisionDeny)
		})
	}
	assertCommandDecision(t, p, "rm harmless.txt", DecisionAllow)
}
