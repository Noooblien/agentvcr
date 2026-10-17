package sandbox

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	kernelEscapePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(/proc/kcore|/proc/sys/|/sys/kernel/|/dev/mem|/dev/kmem)`),
		regexp.MustCompile(`(?i)\b(nsenter|pivot_root|unshare\s+--mount)\b`),
		regexp.MustCompile(`(?i)\bmount\s+.*(/sys|/proc|/dev)\b`),
	}

	destructiveCmdPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f[a-zA-Z]*|-[a-zA-Z]*f[a-zA-Z]*r[a-zA-Z]*)\s+(/|/\*|~|~/\*)(\s+|$)`),
		regexp.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`), // Fork bomb
		regexp.MustCompile(`(?i)\bmkfs(\.[a-zA-Z0-9]+)?\s+/dev/`),
		regexp.MustCompile(`(?i)\bdd\s+if=/dev/zero\s+of=/dev/`),
		regexp.MustCompile(`(?i)>\s*/dev/sd[a-z]`),
	}

	credentialTheftPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(/\.ssh/(id_rsa|id_ed25519|known_hosts)|/etc/shadow|/\.aws/credentials|/\.config/gh/hosts\.yml)`),
		regexp.MustCompile(`(?i)\b(env|printenv|export)\b.*\|\s*(curl|wget|nc|netcat)\b`),
	}

	cloudMetadataPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(169\.254\.169\.254|metadata\.google\.internal|100\.100\.100\.200)`),
		regexp.MustCompile(`(?i)(http://169\.254\.|http://metadata\.)`),
	}
)

// SecurityAuditor inspects command invocations, filesystem paths, and network destinations.
type SecurityAuditor struct {
	config SandboxConfig
}

// NewSecurityAuditor creates an auditor configured with sandbox policies.
func NewSecurityAuditor(config SandboxConfig) *SecurityAuditor {
	return &SecurityAuditor{config: config}
}

// InspectCommand checks a command line for dangerous operations and sandbox escapes.
func (sa *SecurityAuditor) InspectCommand(cmdLine string) []SecurityViolation {
	var violations []SecurityViolation
	now := time.Now().UTC()

	// 1. Cloud Metadata SSRF
	for _, pat := range cloudMetadataPatterns {
		if pat.MatchString(cmdLine) {
			violations = append(violations, SecurityViolation{
				Timestamp:   now,
				Type:        ViolationCloudMetadataSSRF,
				Severity:    SeverityCritical,
				Target:      cmdLine,
				Description: "Attempted access to Cloud Instance Metadata Service (IMDS/SSRF)",
				ActionTaken: "BLOCKED",
			})
			break
		}
	}

	// 2. Kernel Escape
	for _, pat := range kernelEscapePatterns {
		if pat.MatchString(cmdLine) {
			violations = append(violations, SecurityViolation{
				Timestamp:   now,
				Type:        ViolationKernelEscape,
				Severity:    SeverityCritical,
				Target:      cmdLine,
				Description: "Attempted privileged kernel or host namespace escape",
				ActionTaken: "BLOCKED",
			})
			break
		}
	}

	// 3. Destructive Command
	for _, pat := range destructiveCmdPatterns {
		if pat.MatchString(cmdLine) {
			violations = append(violations, SecurityViolation{
				Timestamp:   now,
				Type:        ViolationDestructiveCommand,
				Severity:    SeverityCritical,
				Target:      cmdLine,
				Description: "Attempted unrecoverable host or root filesystem wipe/fork-bomb",
				ActionTaken: "BLOCKED",
			})
			break
		}
	}

	// 4. Credential Theft
	for _, pat := range credentialTheftPatterns {
		if pat.MatchString(cmdLine) {
			violations = append(violations, SecurityViolation{
				Timestamp:   now,
				Type:        ViolationCredentialTheft,
				Severity:    SeverityHigh,
				Target:      cmdLine,
				Description: "Attempted read or exfiltration of sensitive credentials or environment keys",
				ActionTaken: "BLOCKED",
			})
			break
		}
	}

	return violations
}

// InspectNetworkDestination verifies outbound network requests against configured NetworkPolicy.
func (sa *SecurityAuditor) InspectNetworkDestination(rawURLOrHost string) *SecurityViolation {
	now := time.Now().UTC()

	host := extractHost(rawURLOrHost)
	if host == "" {
		host = rawURLOrHost
	}

	// Cloud metadata IP is ALWAYS forbidden regardless of policy
	for _, pat := range cloudMetadataPatterns {
		if pat.MatchString(host) {
			return &SecurityViolation{
				Timestamp:   now,
				Type:        ViolationCloudMetadataSSRF,
				Severity:    SeverityCritical,
				Target:      rawURLOrHost,
				Description: "Access to private cloud metadata service is strictly blocked",
				ActionTaken: "BLOCKED",
			}
		}
	}

	switch sa.config.NetworkPolicy {
	case NetworkPolicyNone:
		return &SecurityViolation{
			Timestamp:   now,
			Type:        ViolationNetworkBlockedHost,
			Severity:    SeverityHigh,
			Target:      rawURLOrHost,
			Description: fmt.Sprintf("Outbound network traffic is completely disabled (destination: %s)", host),
			ActionTaken: "BLOCKED",
		}

	case NetworkPolicyAllowList:
		allowed := false
		for _, a := range sa.config.AllowedHosts {
			if strings.EqualFold(host, a) || strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(a)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SecurityViolation{
				Timestamp:   now,
				Type:        ViolationNetworkBlockedHost,
				Severity:    SeverityHigh,
				Target:      rawURLOrHost,
				Description: fmt.Sprintf("Host '%s' is not in the sandbox allowed hosts whitelist", host),
				ActionTaken: "BLOCKED",
			}
		}

	case NetworkPolicyDenyList:
		for _, b := range sa.config.BlockedHosts {
			if strings.EqualFold(host, b) || strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(b)) {
				return &SecurityViolation{
					Timestamp:   now,
					Type:        ViolationNetworkBlockedHost,
					Severity:    SeverityHigh,
					Target:      rawURLOrHost,
					Description: fmt.Sprintf("Host '%s' is in the sandbox blocked hosts blacklist", host),
					ActionTaken: "BLOCKED",
				}
			}
		}
	}

	return nil
}

func extractHost(target string) string {
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	h := u.Hostname()
	return strings.ToLower(strings.TrimSpace(h))
}
