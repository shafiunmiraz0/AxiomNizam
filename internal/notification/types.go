package notification

// Type aliases re-exporting from models/ so existing code compiles unchanged.

import "axiomnizam.bitbd.net/axiomnizam/internal/notification/models"

const (
	ChannelKind       = models.ChannelKind
	ChannelAPIVersion = models.ChannelAPIVersion
)

type ChannelSpec = models.ChannelSpec
type ChannelResourceStatus = models.ChannelResourceStatus
type ChannelResource = models.ChannelResource
