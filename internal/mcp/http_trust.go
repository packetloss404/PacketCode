package mcp

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/packetcode/packetcode/internal/tools"
)

// AddressClass is the security-relevant class of a resolved network address.
// DNS names are deliberately not a class: every address returned by DNS must
// be classified and checked again immediately before a connection is made.
type AddressClass string

const (
	AddressLoopback  AddressClass = "loopback"
	AddressPrivate   AddressClass = "private"
	AddressLinkLocal AddressClass = "link-local"
	AddressReserved  AddressClass = "reserved"
	AddressPublic    AddressClass = "public"
)

// RedirectMode states the only redirect behaviours permitted by the v1
// Streamable HTTP trust contract.
type RedirectMode string

const (
	RedirectDeny       RedirectMode = "deny"
	RedirectSameOrigin RedirectMode = "same-origin"
)

// RemoteApprovalScope pins the initial remote MCP implementation to one
// approval per call. A user can still deliberately install an exact-tool
// session rule from the approval UI; no server-wide remembered grant exists.
type RemoteApprovalScope string

const ApprovalPerCall RemoteApprovalScope = "call"

// RemoteReconnectMode pins the initial remote MCP implementation to explicit
// reconnects. This prevents a recovery path from silently changing origin.
type RemoteReconnectMode string

const ReconnectManual RemoteReconnectMode = "manual"

// RemoteCredentialMode makes "no credential" distinguishable from an
// omitted credential decision.
type RemoteCredentialMode string

const (
	CredentialNone      RemoteCredentialMode = "none"
	CredentialBearerEnv RemoteCredentialMode = "bearer-env"
)

// RemoteProxyMode is explicit because Go's ambient HTTP(S)_PROXY behaviour
// would let a proxy resolve or receive traffic outside the validated dial path.
type RemoteProxyMode string

const ProxyDisabled RemoteProxyMode = "disabled"

type RemoteTLSMode string

const (
	TLSSystemRoots RemoteTLSMode = "system-roots"
	TLSNone        RemoteTLSMode = "none"
)

type RemoteCompressionMode string

const CompressionIdentity RemoteCompressionMode = "identity"

// RemoteResourceLimits bound every attacker-controlled representation. A
// future streaming client must enforce response/event limits while reading and
// MaxOutputBytes after redaction but before persistence/model delivery.
type RemoteResourceLimits struct {
	MaxResponseBytes int64
	MaxEventBytes    int
	MaxHeaderBytes   int
	MaxOutputBytes   int
}

const RemoteOutputProvenance = "untrusted-remote-mcp"

const (
	minRemoteTimeout         = time.Second
	maxRemoteTimeout         = 5 * time.Minute
	maxRemoteServerNameBytes = 64
	minRemoteCredentialBytes = 16
	maxRemoteCredentialBytes = 4 * 1024
	maxRemoteRedirectHops    = 5
	remoteOutputTruncated    = "\n[REMOTE MCP OUTPUT TRUNCATED]"
	remoteOutputSuffix       = "\n[END UNTRUSTED REMOTE MCP OUTPUT]"
)

var (
	safeServerNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	safeEnvNameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	safeBearerRE     = regexp.MustCompile(`^[-A-Za-z0-9._~+/]+=*$`)

	bearerSecretRE = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	credentialKVRE = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|token|secret|password)(["']?\s*[:=]\s*["']?)[^"',&\s}]+`)
	authHeaderRE   = regexp.MustCompile(`(?i)\b(authorization|proxy-authorization|x-api-key)(["']?\s*:\s*["']?)(?:basic\s+|bearer\s+)?[^"',;\s}]+`)
	cookieHeaderRE = regexp.MustCompile(`(?i)\b(set-cookie|cookie)(["']?\s*:\s*["']?)[^\r\n}]+`)
	urlUserInfoRE  = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
	urlQueryKeyRE  = regexp.MustCompile(`(?i)([?&](?:api(?:[_-]|%5f|%2d)?key|access(?:[_-]|%5f|%2d)?token|refresh(?:[_-]|%5f|%2d)?token|id(?:[_-]|%5f|%2d)?token|client(?:[_-]|%5f|%2d)?secret|token|secret|password)=)[^&#\s]+`)
)

// RemoteHTTPTrustContract is the reviewed, transport-independent security
// input for one future Streamable HTTP MCP server. It is intentionally not
// part of config.MCPServerConfig yet, so defining and testing this contract
// does not enable a network transport or accept a transport flag.
type RemoteHTTPTrustContract struct {
	ServerName            string
	Endpoint              string
	AllowedOrigins        []string
	AllowedAddressClasses []AddressClass
	AllowPlainHTTP        bool
	RedirectMode          RedirectMode
	MaxRedirectHops       int
	ProxyMode             RemoteProxyMode
	TLSMode               RemoteTLSMode
	CompressionMode       RemoteCompressionMode
	CredentialMode        RemoteCredentialMode
	CredentialEnv         string
	ApprovalScope         RemoteApprovalScope
	Timeout               time.Duration
	ReconnectMode         RemoteReconnectMode
	MaxReconnectAttempts  int
	Limits                RemoteResourceLimits
}

// ValidatedRemoteHTTPTrust is the canonical, fail-closed form consumed by a
// future HTTP client. Its methods are the checks a dial/redirect path must use.
type ValidatedRemoteHTTPTrust struct {
	serverName     string
	endpoint       *url.URL
	targetOrigin   string
	allowedOrigins map[string]struct{}
	allowedClasses map[AddressClass]struct{}
	redirectMode   RedirectMode
	maxRedirects   int
	proxyMode      RemoteProxyMode
	tlsMode        RemoteTLSMode
	compression    RemoteCompressionMode
	credentialMode RemoteCredentialMode
	credentialEnv  string
	approvalScope  RemoteApprovalScope
	timeout        time.Duration
	reconnectMode  RemoteReconnectMode
	maxReconnects  int
	limits         RemoteResourceLimits
}

// ValidateRemoteHTTPTrust validates every security decision before any
// network activity. Missing decisions are errors rather than defaults.
func ValidateRemoteHTTPTrust(contract RemoteHTTPTrustContract) (*ValidatedRemoteHTTPTrust, error) {
	name := strings.TrimSpace(contract.ServerName)
	if len(name) == 0 || len(name) > maxRemoteServerNameBytes || !safeServerNameRE.MatchString(name) {
		return nil, fmt.Errorf("remote MCP server name must be 1-%d bytes using only letters, digits, '_' or '-'", maxRemoteServerNameBytes)
	}

	endpoint, targetOrigin, targetIP, err := parseEndpoint(contract.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("remote MCP endpoint: %w", err)
	}

	classes := make(map[AddressClass]struct{}, len(contract.AllowedAddressClasses))
	for _, class := range contract.AllowedAddressClasses {
		if !validAddressClass(class) {
			return nil, fmt.Errorf("remote MCP allowed address class %q is invalid", class)
		}
		classes[class] = struct{}{}
	}
	if len(classes) == 0 {
		return nil, fmt.Errorf("remote MCP allowed address classes must be explicit")
	}
	if targetIP.IsValid() {
		class := ClassifyAddress(targetIP)
		if _, ok := classes[class]; !ok {
			return nil, fmt.Errorf("remote MCP target address class %q is not allowed", class)
		}
	}

	origins := make(map[string]struct{}, len(contract.AllowedOrigins))
	for _, raw := range contract.AllowedOrigins {
		origin, ip, err := parseOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("remote MCP allowed origin %q: %w", raw, err)
		}
		if _, duplicate := origins[origin]; duplicate {
			return nil, fmt.Errorf("remote MCP allowed origin %q is duplicated", origin)
		}
		if ip.IsValid() {
			class := ClassifyAddress(ip)
			if _, ok := classes[class]; !ok {
				return nil, fmt.Errorf("remote MCP origin %q uses disallowed address class %q", origin, class)
			}
		}
		origins[origin] = struct{}{}
	}
	if _, ok := origins[targetOrigin]; !ok {
		return nil, fmt.Errorf("remote MCP target origin %q is not explicitly allowlisted", targetOrigin)
	}

	switch contract.RedirectMode {
	case RedirectDeny:
		if len(origins) != 1 {
			return nil, fmt.Errorf("remote MCP redirect mode %q cannot declare additional origins", contract.RedirectMode)
		}
		if contract.MaxRedirectHops != 0 {
			return nil, fmt.Errorf("remote MCP redirect mode %q requires max redirect hops 0", contract.RedirectMode)
		}
	case RedirectSameOrigin:
		if len(origins) != 1 {
			return nil, fmt.Errorf("remote MCP redirect mode %q cannot declare additional origins", contract.RedirectMode)
		}
		if contract.MaxRedirectHops < 1 || contract.MaxRedirectHops > maxRemoteRedirectHops {
			return nil, fmt.Errorf("remote MCP same-origin redirects require max redirect hops between 1 and %d", maxRemoteRedirectHops)
		}
	default:
		return nil, fmt.Errorf("remote MCP redirect mode must be explicit (deny or same-origin)")
	}

	if err := validatePlainHTTP(endpoint.Scheme, classes, contract.AllowPlainHTTP); err != nil {
		return nil, err
	}
	for origin := range origins {
		u, _ := url.Parse(origin)
		if err := validatePlainHTTP(u.Scheme, classes, contract.AllowPlainHTTP); err != nil {
			return nil, fmt.Errorf("remote MCP origin %q: %w", origin, err)
		}
	}
	if contract.ProxyMode != ProxyDisabled {
		return nil, fmt.Errorf("remote MCP proxy mode must be %q", ProxyDisabled)
	}
	switch endpoint.Scheme {
	case "https":
		if contract.TLSMode != TLSSystemRoots {
			return nil, fmt.Errorf("remote MCP HTTPS requires TLS mode %q", TLSSystemRoots)
		}
	case "http":
		if contract.TLSMode != TLSNone {
			return nil, fmt.Errorf("remote MCP plain HTTP requires TLS mode %q", TLSNone)
		}
	}
	if contract.CompressionMode != CompressionIdentity {
		return nil, fmt.Errorf("remote MCP compression mode must be %q", CompressionIdentity)
	}
	if err := validateRemoteLimits(contract.Limits); err != nil {
		return nil, err
	}
	if err := validateRemoteOutputEnvelope(name, targetOrigin, contract.Limits.MaxOutputBytes); err != nil {
		return nil, err
	}

	credentialEnv := strings.TrimSpace(contract.CredentialEnv)
	switch contract.CredentialMode {
	case CredentialNone:
		if credentialEnv != "" {
			return nil, fmt.Errorf("remote MCP credential mode %q cannot declare credential_env", contract.CredentialMode)
		}
	case CredentialBearerEnv:
		if !safeEnvNameRE.MatchString(credentialEnv) {
			return nil, fmt.Errorf("remote MCP credential mode %q requires a valid credential_env name", contract.CredentialMode)
		}
	default:
		return nil, fmt.Errorf("remote MCP credential mode must be explicit (none or bearer-env)")
	}
	if contract.ApprovalScope != ApprovalPerCall {
		return nil, fmt.Errorf("remote MCP approval scope must be %q", ApprovalPerCall)
	}
	if contract.Timeout < minRemoteTimeout || contract.Timeout > maxRemoteTimeout {
		return nil, fmt.Errorf("remote MCP timeout must be between %s and %s", minRemoteTimeout, maxRemoteTimeout)
	}
	if contract.ReconnectMode != ReconnectManual {
		return nil, fmt.Errorf("remote MCP reconnect mode must be %q", ReconnectManual)
	}
	if contract.MaxReconnectAttempts != 0 {
		return nil, fmt.Errorf("remote MCP manual reconnect mode requires max reconnect attempts 0")
	}

	return &ValidatedRemoteHTTPTrust{
		serverName:     name,
		endpoint:       cloneURL(endpoint),
		targetOrigin:   targetOrigin,
		allowedOrigins: origins,
		allowedClasses: classes,
		redirectMode:   contract.RedirectMode,
		maxRedirects:   contract.MaxRedirectHops,
		proxyMode:      contract.ProxyMode,
		tlsMode:        contract.TLSMode,
		compression:    contract.CompressionMode,
		credentialMode: contract.CredentialMode,
		credentialEnv:  credentialEnv,
		approvalScope:  contract.ApprovalScope,
		timeout:        contract.Timeout,
		reconnectMode:  contract.ReconnectMode,
		maxReconnects:  contract.MaxReconnectAttempts,
		limits:         contract.Limits,
	}, nil
}

func validateRemoteLimits(limits RemoteResourceLimits) error {
	if limits.MaxResponseBytes < 1024 || limits.MaxResponseBytes > 32*1024*1024 {
		return fmt.Errorf("remote MCP max response bytes must be between 1024 and 33554432")
	}
	if limits.MaxEventBytes < 1024 || int64(limits.MaxEventBytes) > limits.MaxResponseBytes || limits.MaxEventBytes > 1024*1024 {
		return fmt.Errorf("remote MCP max event bytes must be between 1024 and min(max response bytes, 1048576)")
	}
	if limits.MaxHeaderBytes < 1024 || limits.MaxHeaderBytes > 128*1024 {
		return fmt.Errorf("remote MCP max header bytes must be between 1024 and 131072")
	}
	if limits.MaxOutputBytes < 1024 || int64(limits.MaxOutputBytes) > limits.MaxResponseBytes || limits.MaxOutputBytes > 1024*1024 {
		return fmt.Errorf("remote MCP max output bytes must be between 1024 and min(max response bytes, 1048576)")
	}
	return nil
}

func validateRemoteOutputEnvelope(serverName, origin string, maxBytes int) error {
	overhead := len(remoteOutputPrefix(serverName, origin)) + len(remoteOutputTruncated) + len(remoteOutputSuffix)
	if overhead > maxBytes {
		return fmt.Errorf("remote MCP max output bytes %d cannot contain the required trust envelope (%d bytes)", maxBytes, overhead)
	}
	return nil
}

func validatePlainHTTP(scheme string, classes map[AddressClass]struct{}, allowed bool) error {
	if scheme != "http" {
		return nil
	}
	if !allowed {
		return fmt.Errorf("remote MCP plain HTTP requires an explicit allow_plain_http decision")
	}
	if _, public := classes[AddressPublic]; public {
		return fmt.Errorf("remote MCP plain HTTP cannot allow public addresses")
	}
	return nil
}

// Endpoint returns a defensive copy of the validated endpoint URL.
func (v *ValidatedRemoteHTTPTrust) Endpoint() *url.URL {
	if v == nil {
		return nil
	}
	return cloneURL(v.endpoint)
}

func (v *ValidatedRemoteHTTPTrust) TargetOrigin() string {
	if v == nil {
		return ""
	}
	return v.targetOrigin
}

func (v *ValidatedRemoteHTTPTrust) ServerName() string {
	if v == nil {
		return ""
	}
	return v.serverName
}

// CredentialEnv returns only the environment variable name. The validator
// never reads or stores its secret value.
func (v *ValidatedRemoteHTTPTrust) CredentialEnv() string {
	if v == nil {
		return ""
	}
	return v.credentialEnv
}

func (v *ValidatedRemoteHTTPTrust) CredentialMode() RemoteCredentialMode {
	if v == nil {
		return ""
	}
	return v.credentialMode
}

func (v *ValidatedRemoteHTTPTrust) ProxyMode() RemoteProxyMode {
	if v == nil {
		return ""
	}
	return v.proxyMode
}

func (v *ValidatedRemoteHTTPTrust) TLSMode() RemoteTLSMode {
	if v == nil {
		return ""
	}
	return v.tlsMode
}

func (v *ValidatedRemoteHTTPTrust) CompressionMode() RemoteCompressionMode {
	if v == nil {
		return ""
	}
	return v.compression
}

func (v *ValidatedRemoteHTTPTrust) ResourceLimits() RemoteResourceLimits {
	if v == nil {
		return RemoteResourceLimits{}
	}
	return v.limits
}

func (v *ValidatedRemoteHTTPTrust) ApprovalScope() RemoteApprovalScope {
	if v == nil {
		return ""
	}
	return v.approvalScope
}

func (v *ValidatedRemoteHTTPTrust) Timeout() time.Duration {
	if v == nil {
		return 0
	}
	return v.timeout
}

func (v *ValidatedRemoteHTTPTrust) ReconnectPolicy() (RemoteReconnectMode, int) {
	if v == nil {
		return "", 0
	}
	return v.reconnectMode, v.maxReconnects
}

func (v *ValidatedRemoteHTTPTrust) AllowedAddressClasses() []AddressClass {
	if v == nil {
		return nil
	}
	out := make([]AddressClass, 0, len(v.allowedClasses))
	for class := range v.allowedClasses {
		out = append(out, class)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// AddressAllowed must be applied to every literal or DNS-resolved address
// immediately before dialing. Mixed DNS answers do not inherit trust from
// one allowed address; each candidate is checked independently.
func (v *ValidatedRemoteHTTPTrust) AddressAllowed(addr netip.Addr) bool {
	if v == nil || !addr.IsValid() {
		return false
	}
	_, ok := v.allowedClasses[ClassifyAddress(addr)]
	return ok
}

// ResolvedAddressesAllowed rejects an empty or mixed-trust DNS answer. A
// caller must not select the convenient address from a response that also
// contains a disallowed rebinding target.
func (v *ValidatedRemoteHTTPTrust) ResolvedAddressesAllowed(addresses []netip.Addr) bool {
	if v == nil || len(addresses) == 0 {
		return false
	}
	for _, addr := range addresses {
		if !v.AddressAllowed(addr) {
			return false
		}
	}
	return true
}

// OriginAllowed validates and checks the exact scheme/host/port tuple.
func (v *ValidatedRemoteHTTPTrust) OriginAllowed(rawURL string) bool {
	if v == nil {
		return false
	}
	_, origin, _, err := parseRequestOrigin(rawURL)
	if err != nil {
		return false
	}
	_, ok := v.allowedOrigins[origin]
	return ok
}

// RemoteRedirectAttempt carries the state that must be checked before one
// redirect. V1 permits only bodyless GET/HEAD continuation; MCP POST redirects
// are refused rather than replaying an approved tool call.
type RemoteRedirectAttempt struct {
	FromURL         string
	ToURL           string
	StatusCode      int
	Hop             int
	PreviousMethod  string
	PreviousHasBody bool
	NextMethod      string
	NextHasBody     bool
}

// RedirectAllowed checks both ends and the complete replay context for one
// redirect hop. A caller must invoke it for every hop; the standard library's
// ambient redirect behaviour is not sufficient for this contract.
func (v *ValidatedRemoteHTTPTrust) RedirectAllowed(attempt RemoteRedirectAttempt) error {
	if v == nil {
		return fmt.Errorf("remote MCP trust contract is nil")
	}
	_, fromOrigin, _, err := parseRequestOrigin(attempt.FromURL)
	if err != nil {
		return fmt.Errorf("remote MCP redirect source: %w", err)
	}
	_, toOrigin, _, err := parseRequestOrigin(attempt.ToURL)
	if err != nil {
		return fmt.Errorf("remote MCP redirect destination: %w", err)
	}
	if _, ok := v.allowedOrigins[fromOrigin]; !ok {
		return fmt.Errorf("remote MCP redirect source origin %q is not allowlisted", fromOrigin)
	}
	if _, ok := v.allowedOrigins[toOrigin]; !ok {
		return fmt.Errorf("remote MCP redirect destination origin %q is not allowlisted", toOrigin)
	}
	switch v.redirectMode {
	case RedirectDeny:
		return fmt.Errorf("remote MCP redirects are disabled")
	case RedirectSameOrigin:
		if fromOrigin != toOrigin {
			return fmt.Errorf("remote MCP cross-origin redirect from %q to %q is denied", fromOrigin, toOrigin)
		}
		if attempt.Hop < 1 || attempt.Hop > v.maxRedirects {
			return fmt.Errorf("remote MCP redirect hop %d exceeds configured maximum %d", attempt.Hop, v.maxRedirects)
		}
		switch attempt.StatusCode {
		case 301, 302, 303, 307, 308:
		default:
			return fmt.Errorf("remote MCP redirect status %d is invalid", attempt.StatusCode)
		}
		previousMethod := strings.ToUpper(strings.TrimSpace(attempt.PreviousMethod))
		nextMethod := strings.ToUpper(strings.TrimSpace(attempt.NextMethod))
		if previousMethod != nextMethod || previousMethod != "GET" && previousMethod != "HEAD" || attempt.PreviousHasBody || attempt.NextHasBody {
			return fmt.Errorf("remote MCP redirect replay is denied (previous=%q next=%q previous_body=%t next_body=%t)", previousMethod, nextMethod, attempt.PreviousHasBody, attempt.NextHasBody)
		}
		return nil
	default:
		return fmt.Errorf("remote MCP redirect mode %q is invalid", v.redirectMode)
	}
}

// ClassifyAddress separates the address categories that must never be
// collapsed into one generic "non-public" trust decision.
func ClassifyAddress(addr netip.Addr) AddressClass {
	if !addr.IsValid() {
		return AddressReserved
	}
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return AddressLoopback
	}
	if addr.IsPrivate() {
		return AddressPrivate
	}
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return AddressLinkLocal
	}
	if addr.Is6() && !publicIPv6Space.Contains(addr) {
		return AddressReserved
	}
	if !addr.IsGlobalUnicast() || addressInReservedRange(addr) {
		return AddressReserved
	}
	return AddressPublic
}

var publicIPv6Space = netip.MustParsePrefix("2000::/3")

func addressInReservedRange(addr netip.Addr) bool {
	for _, prefix := range reservedAddressPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

var reservedAddressPrefixes = mustPrefixes(
	"0.0.0.0/8",
	"100.64.0.0/10",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"240.0.0.0/4",
	"2001::/23",
	"2001:db8::/32",
	"2002::/16",
	"3fff::/20",
	"fec0::/10",
)

func mustPrefixes(raw ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(raw))
	for i, value := range raw {
		out[i] = netip.MustParsePrefix(value)
	}
	return out
}

func validAddressClass(class AddressClass) bool {
	switch class {
	case AddressLoopback, AddressPrivate, AddressLinkLocal, AddressReserved, AddressPublic:
		return true
	default:
		return false
	}
}

func parseEndpoint(raw string) (*url.URL, string, netip.Addr, error) {
	u, origin, ip, err := parseRequestOrigin(raw)
	if err != nil {
		return nil, "", netip.Addr{}, err
	}
	if u.User != nil {
		return nil, "", netip.Addr{}, fmt.Errorf("userinfo is forbidden")
	}
	if u.RawQuery != "" {
		return nil, "", netip.Addr{}, fmt.Errorf("query strings are forbidden; credentials belong in credential_env")
	}
	if u.Fragment != "" {
		return nil, "", netip.Addr{}, fmt.Errorf("fragments are forbidden")
	}
	return u, origin, ip, nil
}

func parseOrigin(raw string) (string, netip.Addr, error) {
	u, origin, ip, err := parseRequestOrigin(raw)
	if err != nil {
		return "", netip.Addr{}, err
	}
	if u.Path != "" && u.Path != "/" {
		return "", netip.Addr{}, fmt.Errorf("origin must not contain a path")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", netip.Addr{}, fmt.Errorf("origin must not contain a query or fragment")
	}
	return origin, ip, nil
}

func parseRequestOrigin(raw string) (*url.URL, string, netip.Addr, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", netip.Addr{}, fmt.Errorf("URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", netip.Addr{}, fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return nil, "", netip.Addr{}, fmt.Errorf("scheme must be http or https")
	}
	if u.User != nil {
		return nil, "", netip.Addr{}, fmt.Errorf("userinfo is forbidden")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return nil, "", netip.Addr{}, fmt.Errorf("host is required")
	}
	if !asciiHost(host) {
		return nil, "", netip.Addr{}, fmt.Errorf("host must be an ASCII DNS name or IP literal")
	}
	portText := u.Port()
	if portText == "" {
		return nil, "", netip.Addr{}, fmt.Errorf("port must be explicit")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, "", netip.Addr{}, fmt.Errorf("port %q is invalid", portText)
	}

	ip, ipErr := netip.ParseAddr(host)
	if host == "localhost" {
		ip = netip.MustParseAddr("127.0.0.1")
		ipErr = nil
	}
	if ipErr != nil && !validDNSName(host) {
		return nil, "", netip.Addr{}, fmt.Errorf("host %q is not a valid DNS name or IP literal", host)
	}
	origin := scheme + "://" + net.JoinHostPort(host, portText)
	u.Scheme = scheme
	u.Host = net.JoinHostPort(host, portText)
	return u, origin, ip, nil
}

func asciiHost(host string) bool {
	for _, r := range host {
		if r > 127 {
			return false
		}
	}
	return true
}

func validDNSName(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !asciiAlphaNumeric(label[0]) || !asciiAlphaNumeric(label[len(label)-1]) {
			return false
		}
		for i := 1; i < len(label)-1; i++ {
			if !asciiAlphaNumeric(label[i]) && label[i] != '-' {
				return false
			}
		}
	}
	return true
}

func asciiAlphaNumeric(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	copy := *u
	return &copy
}

// RedactSensitiveText removes common credential forms from remote MCP output
// and from user-visible MCP logs. It intentionally runs before content is
// persisted or sent to a model by a future network adapter.
func RedactSensitiveText(text string) string {
	text = bearerSecretRE.ReplaceAllString(text, "Bearer [REDACTED]")
	text = authHeaderRE.ReplaceAllString(text, `${1}${2}[REDACTED]`)
	text = cookieHeaderRE.ReplaceAllString(text, `${1}${2}[REDACTED]`)
	text = urlUserInfoRE.ReplaceAllString(text, `${1}[REDACTED]@`)
	text = urlQueryKeyRE.ReplaceAllString(text, `${1}[REDACTED]`)
	return credentialKVRE.ReplaceAllString(text, `${1}${2}[REDACTED]`)
}

func remoteOutputLabel(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(value))
}

func remoteOutputOrigin(origin string) string {
	origin = remoteOutputLabel(origin)
	if _, canonicalOrigin, _, err := parseRequestOrigin(origin); err == nil {
		return canonicalOrigin
	}
	return RedactSensitiveText(origin)
}

// compiledSecretMatcher uses KMP so hostile near-matches stay linear in the
// bounded input plus credential length.
type compiledSecretMatcher struct {
	needle  []byte
	failure []int
}

// buildSecretMatchers precompiles exact-value and base64 candidates. The
// scanners decode mixed percent and JSON escapes while feeding each matcher.
func buildSecretMatchers(secretValues ...string) ([]compiledSecretMatcher, int) {
	candidates := make(map[string]struct{})
	for _, secret := range secretValues {
		if secret == "" {
			continue
		}
		rawCandidates := []string{
			secret,
			base64.StdEncoding.EncodeToString([]byte(secret)),
			base64.RawStdEncoding.EncodeToString([]byte(secret)),
			base64.URLEncoding.EncodeToString([]byte(secret)),
			base64.RawURLEncoding.EncodeToString([]byte(secret)),
		}
		for _, candidate := range rawCandidates {
			if candidate != "" {
				candidates[candidate] = struct{}{}
			}
		}
	}
	ordered := make([]string, 0, len(candidates))
	for candidate := range candidates {
		ordered = append(ordered, candidate)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	matchers := make([]compiledSecretMatcher, 0, len(ordered))
	maxEncodedBytes := 0
	for _, candidate := range ordered {
		matchers = append(matchers, compileSecretMatcher(candidate))
		if encodedBytes := len(candidate) * 6; encodedBytes > maxEncodedBytes {
			maxEncodedBytes = encodedBytes
		}
	}
	return matchers, maxEncodedBytes
}

func compileSecretMatcher(secret string) compiledSecretMatcher {
	needle := []byte(secret)
	failure := make([]int, len(needle))
	for i, matched := 1, 0; i < len(needle); i++ {
		for matched > 0 && needle[i] != needle[matched] {
			matched = failure[matched-1]
		}
		if needle[i] == needle[matched] {
			matched++
		}
		failure[i] = matched
	}
	return compiledSecretMatcher{needle: needle, failure: failure}
}

type remoteTextSpan struct {
	start int
	end   int
}

type decodedScanner func(string, func(byte, int, int))

func redactSensitiveTextWithMatchers(text string, matchers []compiledSecretMatcher) string {
	for _, matcher := range matchers {
		text = redactDecodedSecret(text, matcher, scanPercentDecoded)
		text = redactDecodedSecret(text, matcher, scanJSONDecoded)
	}
	return RedactSensitiveText(text)
}

func redactDecodedSecret(text string, matcher compiledSecretMatcher, scan decodedScanner) string {
	if len(matcher.needle) == 0 || text == "" {
		return text
	}
	ringStarts := make([]int, len(matcher.needle))
	spans := make([]remoteTextSpan, 0, 1)
	matched := 0
	decodedCount := 0
	lastEnd := 0
	scan(text, func(value byte, sourceStart, sourceEnd int) {
		ringIndex := decodedCount % len(matcher.needle)
		ringStarts[ringIndex] = sourceStart
		decodedCount++

		for matched > 0 && value != matcher.needle[matched] {
			matched = matcher.failure[matched-1]
		}
		if value == matcher.needle[matched] {
			matched++
		}
		if matched != len(matcher.needle) {
			return
		}
		startIndex := (decodedCount - len(matcher.needle)) % len(matcher.needle)
		span := remoteTextSpan{start: ringStarts[startIndex], end: sourceEnd}
		if span.start >= lastEnd {
			spans = append(spans, span)
			lastEnd = span.end
		}
		matched = matcher.failure[matched-1]
	})
	if len(spans) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, span := range spans {
		out.WriteString(text[last:span.start])
		out.WriteString("[REDACTED]")
		last = span.end
	}
	out.WriteString(text[last:])
	return out.String()
}

func scanPercentDecoded(text string, emit func(byte, int, int)) {
	for offset := 0; offset < len(text); {
		if text[offset] == '%' && offset+2 < len(text) {
			high, highOK := hexNibble(text[offset+1])
			low, lowOK := hexNibble(text[offset+2])
			if highOK && lowOK {
				emit(high<<4|low, offset, offset+3)
				offset += 3
				continue
			}
		}
		emit(text[offset], offset, offset+1)
		offset++
	}
}

func scanJSONDecoded(text string, emit func(byte, int, int)) {
	for offset := 0; offset < len(text); {
		if text[offset] == '\\' {
			if value, consumed, ok := decodeJSONEscape(text[offset:]); ok {
				var buffer [utf8.UTFMax]byte
				encoded := utf8.AppendRune(buffer[:0], value)
				for _, decoded := range encoded {
					emit(decoded, offset, offset+consumed)
				}
				offset += consumed
				continue
			}
		}
		emit(text[offset], offset, offset+1)
		offset++
	}
}

func decodeJSONEscape(value string) (rune, int, bool) {
	if len(value) < 2 || value[0] != '\\' {
		return 0, 0, false
	}
	switch value[1] {
	case '"', '\\', '/':
		return rune(value[1]), 2, true
	case 'b':
		return '\b', 2, true
	case 'f':
		return '\f', 2, true
	case 'n':
		return '\n', 2, true
	case 'r':
		return '\r', 2, true
	case 't':
		return '\t', 2, true
	case 'u', 'U':
		first, ok := decodeJSONHexRune(value)
		if !ok {
			return 0, 0, false
		}
		if first < 0xD800 || first > 0xDFFF {
			return first, 6, true
		}
		if first > 0xDBFF || len(value) < 12 || value[6] != '\\' || value[7] != 'u' && value[7] != 'U' {
			return 0, 0, false
		}
		second, ok := decodeJSONHexRune(value[6:])
		if !ok || second < 0xDC00 || second > 0xDFFF {
			return 0, 0, false
		}
		return utf16.DecodeRune(first, second), 12, true
	default:
		return 0, 0, false
	}
}

func decodeJSONHexRune(value string) (rune, bool) {
	if len(value) < 6 || value[0] != '\\' || value[1] != 'u' && value[1] != 'U' {
		return 0, false
	}
	var decoded rune
	for i := 2; i < 6; i++ {
		nibble, ok := hexNibble(value[i])
		if !ok {
			return 0, false
		}
		decoded = decoded<<4 | rune(nibble)
	}
	return decoded, true
}

func hexNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

// remoteOutputSanitizer is bound to one validated server and its resolved
// credential value. It is the only output path a future HTTP adapter should
// use, so exact secret redaction and output caps cannot be accidentally omitted.
type remoteOutputSanitizer struct {
	serverName         string
	origin             string
	matchers           []compiledSecretMatcher
	redactionLookahead int
	maxBytes           int
}

// BoundRemoteHTTPTrust atomically couples credential attachment with the
// output sanitizer for one validated server. A future adapter must obtain this
// binding before it can construct an authenticated request.
type BoundRemoteHTTPTrust struct {
	validated *ValidatedRemoteHTTPTrust
	sanitizer *remoteOutputSanitizer
}

func (v *ValidatedRemoteHTTPTrust) BindRuntime(credentialValue string) (*BoundRemoteHTTPTrust, error) {
	if v == nil {
		return nil, fmt.Errorf("remote MCP trust contract is nil")
	}
	switch v.credentialMode {
	case CredentialNone:
		if credentialValue != "" {
			return nil, fmt.Errorf("remote MCP credential mode %q cannot accept a credential value", v.credentialMode)
		}
	case CredentialBearerEnv:
		if len(credentialValue) < minRemoteCredentialBytes || len(credentialValue) > maxRemoteCredentialBytes || !safeBearerRE.MatchString(credentialValue) {
			return nil, fmt.Errorf("remote MCP credential mode %q requires a %d-%d byte header-safe resolved value", v.credentialMode, minRemoteCredentialBytes, maxRemoteCredentialBytes)
		}
	default:
		return nil, fmt.Errorf("remote MCP credential mode %q is invalid", v.credentialMode)
	}
	secrets := []string(nil)
	if credentialValue != "" {
		secrets = []string{credentialValue}
	}
	matchers, lookahead := buildSecretMatchers(secrets...)
	sanitizer := &remoteOutputSanitizer{
		serverName:         v.serverName,
		origin:             v.targetOrigin,
		matchers:           matchers,
		redactionLookahead: lookahead,
		maxBytes:           v.limits.MaxOutputBytes,
	}
	return &BoundRemoteHTTPTrust{validated: v, sanitizer: sanitizer}, nil
}

// CredentialsAllowed returns whether this successfully bound credential may
// be attached to the request's exact origin.
func (b *BoundRemoteHTTPTrust) CredentialsAllowed(rawURL string) bool {
	if b == nil || b.validated == nil || b.sanitizer == nil || b.validated.credentialEnv == "" {
		return false
	}
	_, origin, _, err := parseRequestOrigin(rawURL)
	return err == nil && origin == b.validated.targetOrigin
}

func (b *BoundRemoteHTTPTrust) FormatOutput(content string) string {
	if b == nil {
		return (*remoteOutputSanitizer)(nil).Format(content)
	}
	return b.sanitizer.Format(content)
}

func (b *BoundRemoteHTTPTrust) ToolResult(content string, isError bool) tools.ToolResult {
	if b == nil {
		return (*remoteOutputSanitizer)(nil).ToolResult(content, isError)
	}
	return b.sanitizer.ToolResult(content, isError)
}

func (s *remoteOutputSanitizer) Format(content string) string {
	if s == nil {
		return "[UNTRUSTED REMOTE MCP OUTPUT REFUSED: sanitizer unavailable]"
	}
	prefix := remoteOutputPrefix(s.serverName, s.origin)
	inputLimit := s.maxBytes + s.redactionLookahead
	preTruncated := len(content) > inputLimit
	if preTruncated {
		content = truncateValidUTF8(content, inputLimit)
	}
	content = redactSensitiveTextWithMatchers(content, s.matchers)
	available := s.maxBytes - len(prefix) - len(remoteOutputSuffix)
	if available < 0 {
		available = 0
	}
	if len(content) > available || preTruncated {
		payloadBytes := available - len(remoteOutputTruncated)
		if payloadBytes < 0 {
			payloadBytes = 0
		}
		content = truncateValidUTF8(content, payloadBytes) + remoteOutputTruncated
	}
	return prefix + content + remoteOutputSuffix
}

func remoteOutputPrefix(serverName, origin string) string {
	return fmt.Sprintf("[BEGIN UNTRUSTED REMOTE MCP OUTPUT server=%q origin=%q]\n", remoteOutputLabel(serverName), remoteOutputOrigin(origin))
}

func truncateValidUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "")
	}
	return value
}

// ToolResult keeps sanitized remote content in the provider's tool-role path.
func (s *remoteOutputSanitizer) ToolResult(content string, isError bool) tools.ToolResult {
	if s == nil {
		return tools.ToolResult{
			Content: s.Format(content),
			IsError: true,
			Metadata: map[string]any{
				"provenance": RemoteOutputProvenance,
			},
		}
	}
	return tools.ToolResult{
		Content: s.Format(content),
		IsError: isError,
		Metadata: map[string]any{
			"provenance": RemoteOutputProvenance,
			"server":     remoteOutputLabel(s.serverName),
			"origin":     remoteOutputOrigin(s.origin),
		},
	}
}
