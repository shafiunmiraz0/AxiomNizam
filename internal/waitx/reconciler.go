package waitx

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	wmodels "axiomnizam.bitbd.net/axiomnizam/internal/waitx/models"
)

// WaitCheckReconciler reconciles WaitCheckResource objects.
type WaitCheckReconciler struct{}

// NewWaitCheckReconciler creates a new WaitCheckReconciler.
func NewWaitCheckReconciler() *WaitCheckReconciler {
	return &WaitCheckReconciler{}
}

// Reconcile runs a wait check from a declarative resource.
func (r *WaitCheckReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	check, ok := obj.(*wmodels.WaitCheckResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("expected *WaitCheckResource, got %T", obj)}
	}

	checker, err := buildCheckerFromSpec(check)
	if err != nil {
		return reconciler.ReconcileResult{Error: err}
	}

	checkErr := checker.Check(ctx)
	if checkErr != nil {
		check.Status.LastResult = wmodels.CheckStatusFailed
		check.Status.LastError = checkErr.Error()
		check.Status.TotalFailures++
	} else {
		check.Status.LastResult = wmodels.CheckStatusReady
		check.Status.LastError = ""
		check.Status.TotalSuccesses++
	}
	check.Status.Attempts++
	check.Status.TotalChecks++
	check.Status.LastCheckAt = time.Now().UTC()

	logging.Z().Debug(fmt.Sprintf("waitx reconcile: %s/%s %s -> %s", check.Namespace, check.Name, check.Spec.CheckType, check.Status.LastResult))

	return reconciler.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 30 * time.Second,
	}
}

func buildCheckerFromSpec(check *wmodels.WaitCheckResource) (Checker, error) {
	spec := check.Spec
	opts := spec.Options

	switch spec.CheckType {
	case wmodels.CheckTypeTCP:
		return TCPChecker{Address: spec.Target}, nil
	case wmodels.CheckTypeHTTP:
		return HTTPChecker{
			URL:              spec.Target,
			Method:           opts.Method,
			Headers:          opts.Headers,
			Body:             opts.Body,
			ExpectStatusCode: opts.ExpectStatusCode,
			InsecureSkipTLS:  opts.InsecureSkipTLS,
		}, nil
	case wmodels.CheckTypeDNS:
		return DNSChecker{
			RecordType:     opts.RecordType,
			Address:        spec.Target,
			ExpectedValues: opts.ExpectedValues,
			NameServer:     opts.NameServer,
		}, nil
	case wmodels.CheckTypeGRPC:
		return GRPCHealthChecker{
			Target:             spec.Target,
			Service:            opts.Service,
			UseTLS:             opts.UseTLS,
			TLSServerName:      opts.TLSServerName,
			InsecureSkipVerify: false,
		}, nil
	case wmodels.CheckTypeRedis:
		return RedisChecker{Address: spec.Target, ExpectedKey: opts.ExpectedKey}, nil
	case wmodels.CheckTypeMySQL:
		return MySQLChecker{DSN: opts.DSN, ExpectedTable: opts.ExpectedTable}, nil
	case wmodels.CheckTypePostgreSQL:
		return PostgreSQLChecker{DSN: opts.DSN, ExpectedTable: opts.ExpectedTable}, nil
	case wmodels.CheckTypeMongoDB:
		return MongoDBChecker{URI: opts.URI}, nil
	case wmodels.CheckTypeKafka:
		return KafkaChecker{Brokers: opts.Brokers}, nil
	case wmodels.CheckTypeRabbitMQ:
		return RabbitMQChecker{URL: opts.URL}, nil
	case wmodels.CheckTypeK8sPod:
		return KubernetesPodReadinessChecker{
			PodName:       opts.PodName,
			LabelSelector: opts.LabelSelector,
			Namespace:     opts.Namespace,
			Kubeconfig:    opts.Kubeconfig,
			Context:       opts.KubeContext,
			MinReady:      opts.MinReady,
		}, nil
	default:
		return nil, ErrUnsupportedCheckType
	}
}
