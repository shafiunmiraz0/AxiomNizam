package main

import (
	"context"
	"fmt"
	"os"

	"axiomnizam.bitbd.net/axiomnizam/internal/policies"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

var PolicyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage policies",
	Long:  "Create, list, test, and manage policies",
}

var PolicyApplyCmd = &cobra.Command{
	Use:   "apply -f policy.yaml",
	Short: "Apply a policy",
	Long:  "Apply a policy definition",
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("filename")
		if filename == "" {
			return fmt.Errorf("--filename is required")
		}
		return handlePolicyApply(filename)
	},
}

var PolicyTestCmd = &cobra.Command{
	Use:   "test -f policy.yaml -d data.json",
	Short: "Test a policy",
	Long:  "Test a policy with sample data (dry-run)",
	RunE: func(cmd *cobra.Command, args []string) error {
		policyFile, _ := cmd.Flags().GetString("filename")
		dataFile, _ := cmd.Flags().GetString("data")
		if policyFile == "" {
			return fmt.Errorf("--filename is required")
		}
		return handlePolicyTest(policyFile, dataFile)
	},
}

var PolicyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all policies",
	Long:  "Display all configured policies",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handlePolicyList()
	},
}

var PolicyGetCmd = &cobra.Command{
	Use:   "get [policy-name]",
	Short: "Get a policy",
	Long:  "Display a specific policy and its explanation",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handlePolicyGet(args[0])
	},
}

// handlePolicyApply applies a policy
func handlePolicyApply(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read policy file: %w", err)
	}

	var policy policies.Policy
	if err := yaml.Unmarshal(data, &policy); err != nil {
		return fmt.Errorf("failed to parse policy: %w", err)
	}

	ctx := context.Background()
	if err := policies.AddPolicy(ctx, &policy); err != nil {
		return fmt.Errorf("failed to apply policy: %w", err)
	}

	fmt.Printf("✅ Policy '%s' applied successfully\n", policy.Name)
	fmt.Printf("   Language: %s\n", policy.Language)
	fmt.Printf("   Version: %s\n", policy.Version)
	fmt.Printf("   Effect: %s\n", policy.Effect)

	return nil
}

// handlePolicyTest tests a policy with data
func handlePolicyTest(policyFile, dataFile string) error {
	policyData, err := os.ReadFile(policyFile)
	if err != nil {
		return fmt.Errorf("failed to read policy file: %w", err)
	}

	var policy policies.Policy
	if err := yaml.Unmarshal(policyData, &policy); err != nil {
		return fmt.Errorf("failed to parse policy: %w", err)
	}

	// Parse test data
	testData := make(map[string]interface{})
	if dataFile != "" {
		data, err := os.ReadFile(dataFile)
		if err != nil {
			return fmt.Errorf("failed to read data file: %w", err)
		}

		if err := yaml.Unmarshal(data, &testData); err != nil {
			return fmt.Errorf("failed to parse data: %w", err)
		}
	}

	ctx := context.Background()
	allowed, explanation, err := policies.GlobalPolicyManager.TestPolicy(ctx, policy.Name, testData)

	fmt.Printf("🧪 Policy Test: %s\n", policy.Name)
	fmt.Printf("   Result: %v\n", allowed)
	fmt.Printf("   Explanation:\n%s\n", explanation)

	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	}

	return nil
}

// handlePolicyList lists all policies
func handlePolicyList() error {
	policies := policies.GlobalPolicyManager.ListPolicies()

	fmt.Println("📋 Configured Policies\n")
	fmt.Printf("%-25s %-15s %-10s %-10s %s\n", "NAME", "LANGUAGE", "EFFECT", "ENABLED", "VERSION")
	fmt.Println(repeatString("─", 75))

	for _, p := range policies {
		enabled := "yes"
		if !p.Enabled {
			enabled = "no"
		}
		fmt.Printf("%-25s %-15s %-10s %-10s %s\n", p.Name, p.Language, p.Effect, enabled, p.Version)
	}

	return nil
}

// handlePolicyGet gets and explains a policy
func handlePolicyGet(name string) error {
	policy := policies.GlobalPolicyManager.GetPolicy(name)
	if policy == nil {
		return fmt.Errorf("policy not found: %s", name)
	}

	ctx := context.Background()
	explanation, _ := policies.GlobalPolicyManager.GetPolicyExplanation(ctx, name)

	fmt.Printf("📋 Policy: %s\n\n", name)
	fmt.Printf("Description: %s\n", policy.Description)
	fmt.Printf("Language: %s\n", policy.Language)
	fmt.Printf("Effect: %s\n", policy.Effect)
	fmt.Printf("Priority: %d\n", policy.Priority)
	fmt.Printf("Enabled: %v\n\n", policy.Enabled)

	fmt.Println("Explanation:")
	fmt.Println(explanation)

	return nil
}

func init() {
	PolicyApplyCmd.Flags().StringP("filename", "f", "", "Path to policy file")
	PolicyTestCmd.Flags().StringP("filename", "f", "", "Path to policy file")
	PolicyTestCmd.Flags().StringP("data", "d", "", "Path to test data file")

	PolicyCmd.AddCommand(PolicyApplyCmd)
	PolicyCmd.AddCommand(PolicyTestCmd)
	PolicyCmd.AddCommand(PolicyListCmd)
	PolicyCmd.AddCommand(PolicyGetCmd)
}
