package apiscanner

import "axiomnizam.bitbd.net/axiomnizam/internal/apiscanner/models"

const (
	APIScanKind       = models.APIScanKind
	APIScanAPIVersion = models.APIScanAPIVersion
)

type OutputFormat = models.OutputFormat
type VulnerabilityType = models.VulnerabilityType
type Endpoint = models.Endpoint
type ScanRequest = models.ScanRequest
type ScanResult = models.ScanResult
type ScanCheckStatus = models.ScanCheckStatus
type Finding = models.Finding
type Summary = models.Summary
type APIScanSpec = models.APIScanSpec
type APIScanResourceStatus = models.APIScanResourceStatus
type APIScanResource = models.APIScanResource

const (
	FormatTable = models.FormatTable
	FormatJSON  = models.FormatJSON

	SeverityCritical = models.SeverityCritical
	SeverityHigh     = models.SeverityHigh
	SeverityMedium   = models.SeverityMedium
	SeverityLow      = models.SeverityLow
	SeverityInfo     = models.SeverityInfo

	VulnAuthBypass      = models.VulnAuthBypass
	VulnSQLInjection    = models.VulnSQLInjection
	VulnNoSQLInjection  = models.VulnNoSQLInjection
	VulnHTTPMethod      = models.VulnHTTPMethod
	VulnSecurityHeaders = models.VulnSecurityHeaders
	VulnParameterTamper = models.VulnParameterTamper
	VulnXSS             = models.VulnXSS

	CheckAuthBypassDetection = models.CheckAuthBypassDetection
	CheckAuthBypassTesting   = models.CheckAuthBypassTesting
	CheckSQLInjection        = models.CheckSQLInjection
	CheckNoSQLInjection      = models.CheckNoSQLInjection
	CheckHTTPMethod          = models.CheckHTTPMethod
	CheckSecurityHeaders     = models.CheckSecurityHeaders
	CheckParameterTampering  = models.CheckParameterTampering
	CheckXSS                 = models.CheckXSS
)
