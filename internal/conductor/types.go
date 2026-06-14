package conductor

// Type aliases re-exported from models/ for backward compatibility.

import "axiomnizam.bitbd.net/axiomnizam/internal/conductor/models"

type ProducerConfig = models.ProducerConfig
type ProducerSpec = models.ProducerSpec
type ProducerResourceStatus = models.ProducerResourceStatus
type ProducerResource = models.ProducerResource
type ConsumerConfig = models.ConsumerConfig
type ConsumerSpec = models.ConsumerSpec
type ConsumerResourceStatus = models.ConsumerResourceStatus
type ConsumerResource = models.ConsumerResource

const (
	ProducerKind       = models.ProducerKind
	ProducerAPIVersion = models.ProducerAPIVersion
	ConsumerKind       = models.ConsumerKind
	ConsumerAPIVersion = models.ConsumerAPIVersion
)
