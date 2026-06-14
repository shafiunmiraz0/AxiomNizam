package waitx

import "axiomnizam.bitbd.net/axiomnizam/internal/waitx/models"

// Type aliases for backward compatibility — types live in models/.
type (
	CheckType    = models.CheckType
	CheckStatus  = models.CheckStatus
	RetryPolicy  = models.RetryPolicy
	WaitCheckResource    = models.WaitCheckResource
	WaitCheckSpec        = models.WaitCheckSpec
	WaitCheckStatus      = models.WaitCheckStatus
	CheckOptions         = models.CheckOptions
	CheckGroupResource   = models.CheckGroupResource
	CheckGroupSpec       = models.CheckGroupSpec
	CheckGroupStatus     = models.CheckGroupStatus
)

// Re-export constants.
const (
	CheckTypeTCP        = models.CheckTypeTCP
	CheckTypeHTTP       = models.CheckTypeHTTP
	CheckTypeDNS        = models.CheckTypeDNS
	CheckTypeGRPC       = models.CheckTypeGRPC
	CheckTypeRedis      = models.CheckTypeRedis
	CheckTypeMySQL      = models.CheckTypeMySQL
	CheckTypePostgreSQL = models.CheckTypePostgreSQL
	CheckTypeMongoDB    = models.CheckTypeMongoDB
	CheckTypeRabbitMQ   = models.CheckTypeRabbitMQ
	CheckTypeKafka      = models.CheckTypeKafka
	CheckTypeInfluxDB   = models.CheckTypeInfluxDB
	CheckTypeTemporal   = models.CheckTypeTemporal
	CheckTypeCommand    = models.CheckTypeCommand
	CheckTypeK8sPod     = models.CheckTypeK8sPod

	CheckStatusPending  = models.CheckStatusPending
	CheckStatusRunning  = models.CheckStatusRunning
	CheckStatusReady    = models.CheckStatusReady
	CheckStatusFailed   = models.CheckStatusFailed
	CheckStatusTimeout  = models.CheckStatusTimeout
	CheckStatusInverted = models.CheckStatusInverted

	RetryPolicyLinear      = models.RetryPolicyLinear
	RetryPolicyExponential = models.RetryPolicyExponential
	RetryPolicyFibonacci   = models.RetryPolicyFibonacci
	RetryPolicyCustom      = models.RetryPolicyCustom
)
