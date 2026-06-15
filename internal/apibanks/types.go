package apibanks

import "axiomnizam.bitbd.net/axiomnizam/internal/apibanks/models"

// Re-export domain Resource types from models subpackage.
type APIBankResource = models.APIBankResource
type APIBankSpec = models.APIBankSpec
type APIBankResourceStatus = models.APIBankResourceStatus
type APIReference = models.APIReference

const APIBankKind = models.APIBankKind
const APIBankAPIVersion = models.APIBankAPIVersion
