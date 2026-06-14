package versioning

import "axiomnizam.bitbd.net/axiomnizam/internal/versioning/models"

// Re-export domain Resource types from models subpackage.
type VersionPolicyResource = models.VersionPolicyResource
type VersionPolicySpec = models.VersionPolicySpec
type VersionPolicyResourceStatus = models.VersionPolicyResourceStatus
type RetentionPolicy = models.RetentionPolicy

const VersionPolicyKind = models.VersionPolicyKind
const VersionPolicyAPIVersion = models.VersionPolicyAPIVersion
