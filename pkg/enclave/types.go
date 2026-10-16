package enclave

import (
	"time"
)

// EnclaveSecurityTechnology identifies the underlying hardware Trusted Execution Environment (TEE).
type EnclaveSecurityTechnology string

const (
	TechAMDSEVSNP EnclaveSecurityTechnology = "AMD SEV-SNP (Secure Nested Paging)"
	TechIntelSGX  EnclaveSecurityTechnology = "Intel SGX / TDX (Trust Domain Extensions)"
	TechAWSSNPE   EnclaveSecurityTechnology = "AWS Nitro Enclaves"
	TechSimulated EnclaveSecurityTechnology = "Hardware-Simulated TEE Root-of-Trust"
)

// MeasurementRegister represents a hardware measurement register (PCR / VMSA digest).
type MeasurementRegister struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`   // e.g. "LAUNCH_DIGEST", "MEASUREMENT_PCR0", "ID_KEY_DIGEST"
	Digest string `json:"digest"` // Hex-encoded SHA-384 or SHA-512 measurement
}

// HardwareReportBody captures low-level hardware enclave attestation fields.
type HardwareReportBody struct {
	Technology       EnclaveSecurityTechnology `json:"technology"`
	TCBVersion       string                    `json:"tcb_version"`       // Trusted Computing Base microcode version
	LaunchDigest     string                    `json:"launch_digest"`     // Cryptographic fingerprint of enclave initial memory
	AuthorKeyDigest  string                    `json:"author_key_digest"` // Vendor/Owner signing identity digest
	GuestSVN         uint32                    `json:"guest_svn"`         // Security Version Number
	PolicyFlags      uint64                    `json:"policy_flags"`      // SMT disabled, Debug disabled, Migration restricted
	IsDebuggable     bool                      `json:"is_debuggable"`     // If true, enclave is not production safe
	CPUSerialHash    string                    `json:"cpu_serial_hash"`
	Registers        []MeasurementRegister     `json:"registers"`
}

// EnclaveAttestationQuote represents a complete signed cryptographic quote proving execution inside a secure enclave.
type EnclaveAttestationQuote struct {
	SessionID          string             `json:"session_id"`
	AgentID            string             `json:"agent_id"`
	ReportDataHash     string             `json:"report_data_hash"` // 64-byte payload: SHA-256(trajectory_digest + nonce)
	ReportBody         HardwareReportBody `json:"report_body"`
	PlatformSignature  string             `json:"platform_signature"` // ECDSA-P384 signature by AMD VCEK / Intel PCK
	VCEKCertificate    string             `json:"vcek_certificate"`   // Versioned Chip Endorsement Key X.509 cert
	Nonce              string             `json:"nonce"`
	AttestedAt         time.Time          `json:"attested_at"`
}

// EnclaveVerificationResult contains the verdict of verifying the hardware attestation quote.
type EnclaveVerificationResult struct {
	Valid                 bool   `json:"valid"`
	HardwareSignatureOK   bool   `json:"hardware_signature_ok"`
	CertificateChainOK    bool   `json:"certificate_chain_ok"`
	LaunchMeasurementOK   bool   `json:"launch_measurement_ok"`
	ReportDataBindingOK   bool   `json:"report_data_binding_ok"`
	NonDebugCompliant     bool   `json:"non_debug_compliant"`
	TCBCompliant          bool   `json:"tcb_compliant"`
	FailureReason         string `json:"failure_reason,omitempty"`
}

// EnclaveScorecardReport summarizes the hardware TEE attestation for an agent run.
type EnclaveScorecardReport struct {
	SessionID             string                    `json:"session_id"`
	Quote                 EnclaveAttestationQuote   `json:"quote"`
	Verification          EnclaveVerificationResult `json:"verification"`
	EventCountAttested    int                       `json:"event_count_attested"`
	EnclavePassed         bool                      `json:"enclave_passed"`
	HardwareTrustTier     string                    `json:"hardware_trust_tier"`
	GeneratedAt           time.Time                 `json:"generated_at"`
}
