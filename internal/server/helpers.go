// Package server provides the AxiomNizam server lifecycle and shared helpers.
package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/axiomnizam/internal/auth"
	"example.com/axiomnizam/internal/bootstrapsecrets"
	"example.com/axiomnizam/internal/config"
	"example.com/axiomnizam/internal/database"
	"example.com/axiomnizam/internal/models"
	platformstore "example.com/axiomnizam/internal/platform/store"
	resourcespkg "example.com/axiomnizam/internal/resources"
	"example.com/axiomnizam/internal/workflows"
	clientv3 "go.etcd.io/etcd/client/v3"
	"gorm.io/gorm"
)

const (
	DemoJWTSecretStoreKey = "demo-jwt-secret"
	DemoJWTSecretEtcdKey  = "iam:bootstrap:demo-jwt-secret"
)

// CreateTables runs AutoMigrate on all connected databases.
func CreateTables(conns *database.Connections) {
	if conns.MySQL != nil {
		conns.MySQL.AutoMigrate(&models.User{})
		log.Println("✅ MySQL table created/migrated")
	}
	if conns.MariaDB != nil {
		conns.MariaDB.AutoMigrate(&models.User{})
		log.Println("✅ MariaDB table created/migrated")
	}
	if conns.PostgreSQL != nil {
		conns.PostgreSQL.AutoMigrate(&models.User{})
		log.Println("✅ PostgreSQL table created/migrated")
	}
	if conns.Percona != nil {
		conns.Percona.AutoMigrate(&models.User{})
		log.Println("✅ Percona table created/migrated")
	}
	if conns.Oracle != nil {
		conns.Oracle.AutoMigrate(&models.User{})
		log.Println("✅ Oracle table created/migrated")
	}
}

// EnsureSharedDemoJWTSecret synchronizes the demo JWT secret across replicas.
func EnsureSharedDemoJWTSecret(pg *gorm.DB, etcd *clientv3.Client, kv platformstore.KVStore) (string, error) {
	if configured := strings.TrimSpace(os.Getenv("DEMO_JWT_SECRET")); configured != "" {
		if pg != nil {
			resolved, err := bootstrapsecrets.Ensure(pg, DemoJWTSecretStoreKey, func() (string, error) {
				return configured, nil
			})
			if err != nil {
				log.Printf("⚠️  failed to seed DEMO_JWT_SECRET into postgres bootstrap store: %v", err)
			} else if resolved != configured {
				log.Printf("⚠️  postgres bootstrap DEMO_JWT_SECRET differs from env value; keeping env for current runtime")
			}
		}
		auth.SetDemoJWTSecret(configured)
		return configured, nil
	}

	if pg != nil {
		resolved, err := bootstrapsecrets.Ensure(pg, DemoJWTSecretStoreKey, func() (string, error) {
			return GenerateBootstrapSecret(48)
		})
		if err == nil {
			if err := os.Setenv("DEMO_JWT_SECRET", resolved); err != nil {
				return "", fmt.Errorf("setting DEMO_JWT_SECRET from postgres: %w", err)
			}
			auth.SetDemoJWTSecret(resolved)
			return resolved, nil
		}
		log.Printf("⚠️  postgres bootstrap for DEMO_JWT_SECRET failed, falling back to KV store: %v", err)
	}

	resolved, err := EnsureSharedDemoJWTSecretFromKV(kv, etcd)
	if err != nil {
		return "", err
	}
	if err := os.Setenv("DEMO_JWT_SECRET", resolved); err != nil {
		return "", fmt.Errorf("setting DEMO_JWT_SECRET from KV store: %w", err)
	}
	auth.SetDemoJWTSecret(resolved)
	return resolved, nil
}

// EnsureSharedDemoJWTSecretFromKV uses the KVStore abstraction (works
// with both etcd and Raft backends). Falls back to raw etcd if KV is nil.
func EnsureSharedDemoJWTSecretFromKV(kv platformstore.KVStore, etcd *clientv3.Client) (string, error) {
	if kv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		candidate, err := GenerateBootstrapSecret(48)
		if err != nil {
			return "", err
		}

		resolved, _, err := kv.CAS(ctx, DemoJWTSecretEtcdKey, candidate)
		if err != nil {
			return "", fmt.Errorf("persisting demo token secret via KV store: %w", err)
		}
		if resolved == "" {
			return "", fmt.Errorf("demo token secret CAS returned empty value")
		}
		return resolved, nil
	}

	// Fallback to raw etcd client.
	return EnsureSharedDemoJWTSecretFromEtcd(etcd)
}

// EnsureSharedDemoJWTSecretFromEtcd persists the JWT secret using raw etcd transactions.
func EnsureSharedDemoJWTSecretFromEtcd(etcd *clientv3.Client) (string, error) {
	if etcd == nil {
		return "", fmt.Errorf("DEMO_JWT_SECRET is not set and neither postgres nor etcd bootstrap store is available")
	}

	getCtx, getCancel := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := etcd.Get(getCtx, DemoJWTSecretEtcdKey)
	getCancel()
	if err != nil {
		return "", fmt.Errorf("reading demo token secret from etcd: %w", err)
	}
	if len(resp.Kvs) > 0 {
		secret := strings.TrimSpace(string(resp.Kvs[0].Value))
		if secret != "" {
			return secret, nil
		}
	}

	candidate, err := GenerateBootstrapSecret(48)
	if err != nil {
		return "", err
	}

	txnCtx, txnCancel := context.WithTimeout(context.Background(), 5*time.Second)
	txnResp, err := etcd.Txn(txnCtx).
		If(clientv3.Compare(clientv3.Version(DemoJWTSecretEtcdKey), "=", 0)).
		Then(clientv3.OpPut(DemoJWTSecretEtcdKey, candidate)).
		Else(clientv3.OpGet(DemoJWTSecretEtcdKey)).
		Commit()
	txnCancel()
	if err != nil {
		return "", fmt.Errorf("persisting demo token secret in etcd: %w", err)
	}

	resolved := candidate
	if !txnResp.Succeeded {
		resolved = ""
		if len(txnResp.Responses) > 0 {
			rangeResp := txnResp.Responses[0].GetResponseRange()
			if rangeResp != nil && len(rangeResp.Kvs) > 0 {
				resolved = strings.TrimSpace(string(rangeResp.Kvs[0].Value))
			}
		}
		if resolved == "" {
			return "", fmt.Errorf("demo token secret exists in etcd but value is empty")
		}
	}

	return resolved, nil
}

// GenerateBootstrapSecret generates a cryptographically secure random secret.
func GenerateBootstrapSecret(size int) (string, error) {
	if size <= 0 {
		size = 48
	}
	random := make([]byte, size)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generating bootstrap secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

// ResolveSecurityEnvironment determines the current environment.
func ResolveSecurityEnvironment() string {
	candidates := []string{
		strings.TrimSpace(os.Getenv("AXIOMNIZAM_ENV")),
		strings.TrimSpace(os.Getenv("APP_ENV")),
		strings.TrimSpace(os.Getenv("ENVIRONMENT")),
		strings.TrimSpace(os.Getenv("GO_ENV")),
	}
	for _, c := range candidates {
		if c != "" {
			return strings.ToLower(c)
		}
	}
	return "development"
}

// IsProductionEnvironment checks if the env string indicates production.
func IsProductionEnvironment(env string) bool {
	normalized := strings.ToLower(strings.TrimSpace(env))
	return normalized == "production" || normalized == "prod"
}

// ResolveSecurityGuardrailMode determines the guardrail enforcement mode.
func ResolveSecurityGuardrailMode(env string) string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SECURITY_GUARDRAILS_MODE")))
	switch mode {
	case "off", "audit", "enforce":
		return mode
	case "":
		if IsProductionEnvironment(env) {
			return "audit"
		}
		return "off"
	default:
		log.Printf("⚠️  Unknown SECURITY_GUARDRAILS_MODE=%q, defaulting to audit", mode)
		return "audit"
	}
}

// ApplySecurityGuardrails validates configuration for insecure defaults.
func ApplySecurityGuardrails(cfg *config.Config) {
	if cfg == nil {
		return
	}

	env := ResolveSecurityEnvironment()
	mode := ResolveSecurityGuardrailMode(env)
	if mode == "off" {
		return
	}

	blocking := make([]string, 0)
	addBlocking := func(msg string) {
		if strings.TrimSpace(msg) != "" {
			blocking = append(blocking, msg)
		}
	}

	isDefault := func(value string, defaults ...string) bool {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		for _, d := range defaults {
			if trimmed == strings.ToLower(strings.TrimSpace(d)) {
				return true
			}
		}
		return false
	}

	if isDefault(cfg.IAM.SysadminPassword, "", "change-me", "changeme", "default", "password", "admin") {
		addBlocking("IAM_SYSADMIN_PASSWORD is empty or default-like")
	}
	if strings.TrimSpace(os.Getenv("DEMO_JWT_SECRET")) == "" {
		addBlocking("DEMO_JWT_SECRET is not set")
	}
	if strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")) == "" {
		addBlocking("CORS_ALLOWED_ORIGINS is empty")
	}

	// TRUSTED_PROXIES=0.0.0.0/0 trusts all sources — defeats IP-based controls.
	if tp := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); tp == "0.0.0.0/0" {
		addBlocking("TRUSTED_PROXIES=0.0.0.0/0 trusts all sources; restrict to actual proxy CIDRs")
	}

	if isDefault(cfg.MySQL.Password, "root", "password") {
		addBlocking("MYSQL_PASSWORD is a default credential")
	}
	if isDefault(cfg.PostgreSQL.Password, "postgres", "password") {
		addBlocking("POSTGRES_PASSWORD is a default credential")
	}
	if isDefault(cfg.Oracle.Password, "oracle123", "password") {
		addBlocking("ORACLE_PASSWORD is a default credential")
	}

	if len(blocking) == 0 {
		if mode == "audit" {
			log.Printf("✅ Security guardrails check passed (env=%s, mode=%s)", env, mode)
		}
		return
	}

	for _, b := range blocking {
		log.Printf("🚨 Security guardrail issue: %s", b)
	}

	if mode == "enforce" && IsProductionEnvironment(env) {
		log.Fatalf("❌ Security guardrails blocked startup in production (mode=%s)", mode)
		return
	}

	log.Printf("⚠️  Security guardrails detected %d blocking issue(s) but startup continues (env=%s, mode=%s)", len(blocking), env, mode)
}

// EnsureWorkflowRegistered loads a workflow from the resource store if not already registered.
func EnsureWorkflowRegistered(ctx context.Context, resourceHandler *resourcespkg.GenericResourceHandler, workflowName string) error {
	if resourceHandler == nil {
		if workflows.GlobalWorkflowEngine.GetWorkflow(workflowName) != nil {
			return nil
		}
		return fmt.Errorf("workflow %q not found", workflowName)
	}

	resource, found := resourceHandler.FindResourceByKindAndName("workflow", workflowName)
	if !found {
		if workflows.GlobalWorkflowEngine.GetWorkflow(workflowName) != nil {
			return nil
		}
		return fmt.Errorf("workflow %q not found", workflowName)
	}

	workflowDef, err := WorkflowFromResource(resource)
	if err != nil {
		return err
	}

	return workflows.AddWorkflow(ctx, workflowDef)
}

// WorkflowFromResource converts a GenericResource to a Workflow definition.
func WorkflowFromResource(resource *resourcespkg.GenericResource) (*workflows.Workflow, error) {
	if resource == nil {
		return nil, fmt.Errorf("workflow definition is nil")
	}

	name := strings.TrimSpace(resource.Metadata.Name)
	if name == "" {
		return nil, fmt.Errorf("workflow metadata.name is required")
	}

	steps, err := WorkflowStepsFromSpec(name, resource.Spec)
	if err != nil {
		return nil, err
	}

	enabled := true
	if v, ok := BoolFromAny(resource.Spec["enabled"]); ok {
		enabled = v
	}
	if schedule, ok := resource.Spec["schedule"].(map[string]interface{}); ok {
		if v, ok := BoolFromAny(schedule["enabled"]); ok {
			enabled = v
		}
	}

	version := strings.TrimSpace(StringFromAny(resource.Spec["version"]))
	if version == "" {
		version = "v1"
	}

	namespace := strings.TrimSpace(resource.Metadata.Namespace)
	if namespace == "" {
		namespace = "default"
	}

	return &workflows.Workflow{
		Name:        name,
		Namespace:   namespace,
		Version:     version,
		Description: StringFromAny(resource.Spec["description"]),
		Triggers:    WorkflowTriggersFromSpec(resource.Spec),
		Steps:       steps,
		Enabled:     enabled,
		Labels:      resource.Metadata.Labels,
		Annotations: resource.Metadata.Annotations,
	}, nil
}

// WorkflowTriggersFromSpec extracts workflow triggers from a resource spec.
func WorkflowTriggersFromSpec(spec map[string]interface{}) []workflows.WorkflowTrigger {
	triggers := make([]workflows.WorkflowTrigger, 0)

	if raw, ok := spec["triggers"].([]interface{}); ok {
		for _, item := range raw {
			triggerMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			triggerType := strings.TrimSpace(StringFromAny(triggerMap["type"]))
			if triggerType == "" {
				continue
			}

			condition := make(map[string]interface{})
			if condMap, ok := triggerMap["condition"].(map[string]interface{}); ok {
				for k, v := range condMap {
					condition[k] = v
				}
			}

			triggers = append(triggers, workflows.WorkflowTrigger{
				Type:      triggerType,
				Condition: condition,
			})
		}
	}

	if schedule, ok := spec["schedule"].(map[string]interface{}); ok {
		condition := make(map[string]interface{}, len(schedule))
		for k, v := range schedule {
			condition[k] = v
		}
		triggers = append(triggers, workflows.WorkflowTrigger{Type: "schedule", Condition: condition})
	}

	if len(triggers) == 0 {
		triggers = append(triggers, workflows.WorkflowTrigger{Type: "manual", Condition: map[string]interface{}{"source": "api"}})
	}

	return triggers
}

// WorkflowStepsFromSpec extracts workflow steps from a resource spec.
func WorkflowStepsFromSpec(workflowName string, spec map[string]interface{}) ([]workflows.WorkflowStep, error) {
	rawSteps, ok := spec["steps"].([]interface{})
	if !ok || len(rawSteps) == 0 {
		return nil, fmt.Errorf("workflow %q must define at least one step", workflowName)
	}

	steps := make([]workflows.WorkflowStep, 0, len(rawSteps))
	for i, rawStep := range rawSteps {
		stepMap, ok := rawStep.(map[string]interface{})
		if !ok {
			continue
		}

		stepID := strings.TrimSpace(StringFromAny(stepMap["id"]))
		if stepID == "" {
			stepID = fmt.Sprintf("%s-step-%d", workflowName, i+1)
		}

		stepName := strings.TrimSpace(StringFromAny(stepMap["name"]))
		if stepName == "" {
			stepName = stepID
		}

		stepType := strings.TrimSpace(StringFromAny(stepMap["type"]))
		if stepType == "" {
			stepType = "http"
		}

		action := strings.TrimSpace(StringFromAny(stepMap["action"]))
		if action == "" {
			action = stepType
		}

		stepConfig := make(map[string]interface{})
		if rawConfig, ok := stepMap["config"].(map[string]interface{}); ok {
			for k, v := range rawConfig {
				stepConfig[k] = v
			}
		}

		for k, v := range stepMap {
			switch k {
			case "id", "name", "type", "action", "retry", "timeout", "config":
				continue
			default:
				stepConfig[k] = v
			}
		}

		if _, exists := stepConfig["action"]; !exists && action != "" {
			stepConfig["action"] = action
		}
		if stepType == "http" {
			if _, exists := stepConfig["method"]; !exists {
				method := strings.ToUpper(strings.TrimSpace(StringFromAny(stepMap["method"])))
				if method == "" {
					method = "GET"
				}
				stepConfig["method"] = method
			}
		}

		steps = append(steps, workflows.WorkflowStep{
			ID:      stepID,
			Name:    stepName,
			Type:    stepType,
			Action:  action,
			Config:  stepConfig,
			Timeout: DurationFromAny(stepMap["timeout"]),
			Retry:   IntFromAny(stepMap["retry"]),
		})
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow %q has invalid steps", workflowName)
	}

	return steps, nil
}

// StringFromAny converts an interface{} value to a string.
func StringFromAny(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// BoolFromAny converts an interface{} value to a bool.
func BoolFromAny(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err == nil {
			return parsed, true
		}
	}
	return false, false
}

// IntFromAny converts an interface{} value to an int.
func IntFromAny(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return parsed
		}
	}
	return 0
}

// DurationFromAny converts an interface{} value to a time.Duration.
func DurationFromAny(value interface{}) time.Duration {
	switch v := value.(type) {
	case time.Duration:
		return v
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(v))
		if err == nil {
			return parsed
		}
	case int:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case int64:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case float64:
		if v > 0 {
			return time.Duration(v * float64(time.Second))
		}
	}
	return 0
}
