package anonymization

import "axiomnizam.bitbd.net/axiomnizam/internal/anonymization/models"

// Type aliases re-exported from models/ for backward compatibility.

type MaskTechnique = models.MaskTechnique
type AnonymRule = models.AnonymRule
type PolicyScope = models.PolicyScope
type AnonymizationPolicySpec = models.AnonymizationPolicySpec
type AnonymizationPolicyResourceStatus = models.AnonymizationPolicyResourceStatus
type AnonymizationPolicyResource = models.AnonymizationPolicyResource

const (
	AnonymizationPolicyKind       = models.AnonymizationPolicyKind
	AnonymizationPolicyAPIVersion = models.AnonymizationPolicyAPIVersion

	MaskHash       = models.MaskHash
	MaskRedact     = models.MaskRedact
	MaskPartial    = models.MaskPartial
	MaskTokenize   = models.MaskTokenize
	MaskNoise      = models.MaskNoise
	MaskGeneralize = models.MaskGeneralize
	MaskSynthetic  = models.MaskSynthetic
	MaskShuffle    = models.MaskShuffle
)
