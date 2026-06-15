package netintel

// Type aliases re-exporting from models/ so existing code compiles unchanged.

import "axiomnizam.bitbd.net/axiomnizam/internal/netintel/models"

const (
	ConfigKind       = models.ConfigKind
	ConfigAPIVersion = models.ConfigAPIVersion
)

type ConfigSpec = models.ConfigSpec
type ConfigResourceStatus = models.ConfigResourceStatus
type ConfigResource = models.ConfigResource
