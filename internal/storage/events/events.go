// Package events is a backward-compatible re-export of the storage audit package.
// New code should import "axiomnizam.bitbd.net/axiomnizam/internal/storage/audit" directly.
package events

import "axiomnizam.bitbd.net/axiomnizam/internal/storage/audit"

// Type aliases for backward compatibility.
type AuditLog = audit.AuditLog

// Function aliases for backward compatibility.
var NewAuditLog = audit.NewAuditLog

// Constant aliases for backward compatibility.
const (
	EventBucketCreated        = audit.EventBucketCreated
	EventBucketDeleted        = audit.EventBucketDeleted
	EventObjectUploaded       = audit.EventObjectUploaded
	EventObjectDownloaded     = audit.EventObjectDownloaded
	EventObjectDeleted        = audit.EventObjectDeleted
	EventObjectCopied         = audit.EventObjectCopied
	EventPolicyCreated        = audit.EventPolicyCreated
	EventPolicyDeleted        = audit.EventPolicyDeleted
	EventPresignGenerated     = audit.EventPresignGenerated
	EventMultiDelete          = audit.EventMultiDelete
	EventObjectScanClean      = audit.EventObjectScanClean
	EventObjectThreatDetected = audit.EventObjectThreatDetected
)
