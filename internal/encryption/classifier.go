// Package encryption — Phase 19: Classification scanner.
//
// Crawls model structs and reports fields that look sensitive but lack
// classification tags. This helps identify fields that should be encrypted
// but were missed during manual tagging.
//
// The scanner uses heuristics: field names containing "email", "password",
// "secret", "token", "key", "ssn", "phone", etc. are flagged as potentially
// sensitive.

package encryption

import (
	"fmt"
	"reflect"
	"strings"
)

// SensitiveFieldPatterns are field name substrings that suggest sensitivity.
var SensitiveFieldPatterns = []string{
	"email", "mail",
	"password", "passwd", "pwd",
	"secret", "token", "key", "credential",
	"ssn", "social_security", "tax_id",
	"phone", "mobile", "cell",
	"address", "street", "zip", "postal",
	"dob", "date_of_birth", "birth_date",
	"card", "credit_card", "pan",
	"ip_address", "ip_addr",
	"fingerprint", "device_id",
	"recovery_code", "backup_code",
	"totp", "otp", "mfa",
	"api_key", "access_key", "secret_key",
	"private_key", "signing_key",
	"hash", "salt",
}

// ClassificationReport describes a field that should be classified.
type ClassificationReport struct {
	StructName      string
	FieldName       string
	FieldType       string
	FileHint        string
	SuggestedLevel  string // "PII", "Sensitive", or "Confidential"
	Reason          string // which pattern matched
	HasTag          bool   // true if already classified
	CurrentTagValue string // current classification tag value, if any
}

// ScanStruct analyzes a struct type and returns a report of fields that
// look sensitive but lack classification tags. Pass a pointer to the struct
// type as `model`.
func ScanStruct(model any) []ClassificationReport {
	var reports []ClassificationReport

	v := reflect.ValueOf(model)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reports
	}

	t := v.Type()
	structName := t.Name()

	for i := range t.NumField() {
		field := t.Field(i)
		fieldName := field.Name
		fieldTypeName := field.Type.String()

		// Only analyze string fields (encryption works on strings)
		if field.Type.Kind() != reflect.String {
			continue
		}

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check if already classified
		existingTag := field.Tag.Get("classification")
		if existingTag != "" {
			// Already classified — include in report with HasTag=true
			reports = append(reports, ClassificationReport{
				StructName:      structName,
				FieldName:       fieldName,
				FieldType:       fieldTypeName,
				SuggestedLevel:  existingTag,
				Reason:          "already classified",
				HasTag:          true,
				CurrentTagValue: existingTag,
			})
			continue
		}

		// Check if field name matches sensitive patterns
		lowerName := strings.ToLower(fieldName)
		for _, pattern := range SensitiveFieldPatterns {
			if strings.Contains(lowerName, pattern) {
				level := classifyByPattern(pattern)
				reports = append(reports, ClassificationReport{
					StructName:     structName,
					FieldName:      fieldName,
					FieldType:      fieldTypeName,
					SuggestedLevel: level,
					Reason:         fmt.Sprintf("field name contains '%s'", pattern),
					HasTag:         false,
				})
				break // one match is enough
			}
		}
	}

	return reports
}

// ScanMultipleStructs scans multiple structs and returns combined reports.
// Pass pointers to struct instances: ScanMultipleStructs(&User{}, &Session{}, ...)
func ScanMultipleStructs(models ...any) []ClassificationReport {
	var all []ClassificationReport
	for _, model := range models {
		all = append(all, ScanStruct(model)...)
	}
	return all
}

// UnclassifiedOnly filters a report to only include fields without tags.
func UnclassifiedOnly(reports []ClassificationReport) []ClassificationReport {
	var result []ClassificationReport
	for _, r := range reports {
		if !r.HasTag {
			result = append(result, r)
		}
	}
	return result
}

// classifyByPattern returns the suggested classification level based on
// the matched pattern.
func classifyByPattern(pattern string) string {
	switch pattern {
	case "password", "passwd", "pwd", "secret", "private_key", "signing_key",
		"totp", "otp", "mfa", "recovery_code", "backup_code",
		"api_key", "access_key", "secret_key", "credential":
		return ClassificationConfidential

	case "ssn", "social_security", "tax_id", "card", "credit_card", "pan",
		"dob", "date_of_birth", "birth_date":
		return ClassificationPII

	case "email", "mail", "phone", "mobile", "cell",
		"address", "street", "zip", "postal",
		"ip_address", "ip_addr", "fingerprint", "device_id":
		return ClassificationPII

	case "token", "key", "hash", "salt":
		return ClassificationSensitive

	default:
		return ClassificationSensitive
	}
}

// FormatReport prints a classification report to stdout.
func FormatReport(reports []ClassificationReport) {
	unclassified := UnclassifiedOnly(reports)
	classified := len(reports) - len(unclassified)

	fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  Data Classification Report\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  Total sensitive fields:  %d\n", len(reports))
	fmt.Printf("  Already classified:      %d\n", classified)
	fmt.Printf("  Needs classification:    %d\n", len(unclassified))
	fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")

	if len(unclassified) > 0 {
		fmt.Printf("⚠️  Unclassified sensitive fields:\n\n")
		for _, r := range unclassified {
			fmt.Printf("  %s.%s (%s)\n", r.StructName, r.FieldName, r.FieldType)
			fmt.Printf("    Suggested: %s — %s\n\n", r.SuggestedLevel, r.Reason)
		}
	}

	if classified > 0 {
		fmt.Printf("✅ Already classified fields:\n\n")
		for _, r := range reports {
			if r.HasTag {
				fmt.Printf("  %s.%s → %s\n", r.StructName, r.FieldName, r.CurrentTagValue)
			}
		}
	}
}
