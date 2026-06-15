package main

import (
	"context"
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks"

	"github.com/spf13/cobra"
)

var ApiBankCmd = &cobra.Command{
	Use:   "apibank",
	Short: "Manage API banks",
	Long:  "Create, list, and manage collections of related APIs",
}

var ApiBankCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API bank",
	Long:  "Create a new API bank",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		owner, _ := cmd.Flags().GetString("owner")
		desc, _ := cmd.Flags().GetString("description")

		if name == "" {
			return fmt.Errorf("--name is required")
		}

		return handleCreateAPIBank(name, owner, desc)
	},
}

var ApiBankListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API banks",
	Long:  "Display all API banks",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleListAPIBanks()
	},
}

var ApiBankGetCmd = &cobra.Command{
	Use:   "get [bank-name]",
	Short: "Get API bank details",
	Long:  "Display details of an API bank",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleGetAPIBank(args[0])
	},
}

var ApiBankAddAPICmd = &cobra.Command{
	Use:   "add-api [bank-name]",
	Short: "Add API to bank",
	Long:  "Add an API to an API bank",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiName, _ := cmd.Flags().GetString("api-name")
		endpoint, _ := cmd.Flags().GetString("endpoint")
		kind, _ := cmd.Flags().GetString("kind")

		if apiName == "" || endpoint == "" {
			return fmt.Errorf("--api-name and --endpoint are required")
		}

		return handleAddAPIToBank(args[0], apiName, endpoint, kind)
	},
}

var ApiBankSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search API banks",
	Long:  "Search banks by tag, owner, or data class",
	RunE: func(cmd *cobra.Command, args []string) error {
		tag, _ := cmd.Flags().GetString("tag")
		owner, _ := cmd.Flags().GetString("owner")
		dataClass, _ := cmd.Flags().GetString("data-class")

		return handleSearchAPIBanks(tag, owner, dataClass)
	},
}

// handleCreateAPIBank creates an API bank
func handleCreateAPIBank(name, owner, description string) error {
	bank := &apibanks.APIBank{
		Name:        name,
		Owner:       owner,
		Description: description,
		APIs:        make([]apibanks.APIReference, 0),
		Tags:        make([]string, 0),
	}

	ctx := context.Background()
	if err := apibanks.CreateBank(ctx, bank); err != nil {
		return fmt.Errorf("failed to create API bank: %w", err)
	}

	fmt.Printf("✅ API bank created: %s\n", name)
	fmt.Printf("   Owner: %s\n", owner)
	fmt.Printf("   Description: %s\n", description)

	return nil
}

// handleListAPIBanks lists all API banks
func handleListAPIBanks() error {
	banks := apibanks.ListBanks()

	fmt.Println("📦 API Banks")
	fmt.Println()
	fmt.Printf("%-30s %-20s %-10s %s\n", "NAME", "OWNER", "APIS", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 85))

	for _, bank := range banks {
		fmt.Printf("%-30s %-20s %-10d %s\n",
			bank.Name,
			bank.Owner,
			len(bank.APIs),
			bank.Description,
		)
	}

	return nil
}

// handleGetAPIBank gets details of an API bank
func handleGetAPIBank(name string) error {
	bank := apibanks.GetBank(name)
	if bank == nil {
		return fmt.Errorf("API bank not found: %s", name)
	}

	fmt.Printf("📦 API Bank: %s\n\n", name)
	fmt.Printf("Owner: %s\n", bank.Owner)
	fmt.Printf("Description: %s\n", bank.Description)
	fmt.Printf("Version: %s\n", bank.Version)
	fmt.Printf("APIs: %d\n\n", len(bank.APIs))

	if len(bank.APIs) > 0 {
		fmt.Println("APIs:")
		for i, api := range bank.APIs {
			fmt.Printf("  %d. %s (%s)\n", i+1, api.Name, api.Kind)
			fmt.Printf("     Endpoint: %s\n", api.Endpoint)
			if api.Description != "" {
				fmt.Printf("     Description: %s\n", api.Description)
			}
			if api.SLA != "" {
				fmt.Printf("     SLA: %s\n", api.SLA)
			}
			if len(api.DataClasses) > 0 {
				fmt.Printf("     Data Classes: %v\n", api.DataClasses)
			}
		}
	}

	return nil
}

// handleAddAPIToBank adds an API to a bank
func handleAddAPIToBank(bankName, apiName, endpoint, kind string) error {
	api := apibanks.APIReference{
		Name:     apiName,
		Kind:     kind,
		Endpoint: endpoint,
	}

	ctx := context.Background()
	if err := apibanks.AddAPIToBank(ctx, bankName, api); err != nil {
		return fmt.Errorf("failed to add API: %w", err)
	}

	fmt.Printf("✅ API added to bank: %s\n", apiName)
	fmt.Printf("   Bank: %s\n", bankName)
	fmt.Printf("   Kind: %s\n", kind)
	fmt.Printf("   Endpoint: %s\n", endpoint)

	return nil
}

// handleSearchAPIBanks searches for API banks
func handleSearchAPIBanks(tag, owner, dataClass string) error {
	catalog := apibanks.NewAPIBankCatalog(apibanks.GlobalAPIBankManager)

	fmt.Println("🔍 API Bank Search Results")
	fmt.Println()

	if owner != "" {
		banks := catalog.SearchByOwner(owner)
		fmt.Printf("Banks owned by %s:\n", owner)
		for _, bank := range banks {
			fmt.Printf("  - %s (%d APIs)\n", bank.Name, len(bank.APIs))
		}
		fmt.Println()
	}

	if tag != "" {
		banks := catalog.SearchByTag(tag)
		fmt.Printf("Banks with tag '%s':\n", tag)
		for _, bank := range banks {
			fmt.Printf("  - %s (%d APIs)\n", bank.Name, len(bank.APIs))
		}
		fmt.Println()
	}

	if dataClass != "" {
		apis := catalog.SearchByDataClass(dataClass)
		fmt.Printf("APIs exposing data class '%s':\n", dataClass)
		for _, api := range apis {
			fmt.Printf("  - %s (%s)\n", api.Name, api.Kind)
		}
	}

	return nil
}

func init() {
	ApiBankCreateCmd.Flags().StringP("name", "n", "", "Bank name")
	ApiBankCreateCmd.Flags().StringP("owner", "o", "", "Bank owner")
	ApiBankCreateCmd.Flags().StringP("description", "d", "", "Bank description")

	ApiBankAddAPICmd.Flags().StringP("api-name", "", "", "API name")
	ApiBankAddAPICmd.Flags().StringP("endpoint", "", "", "API endpoint")
	ApiBankAddAPICmd.Flags().StringP("kind", "k", "REST", "API kind (REST, GraphQL, gRPC)")

	ApiBankSearchCmd.Flags().StringP("tag", "t", "", "Search by tag")
	ApiBankSearchCmd.Flags().StringP("owner", "o", "", "Search by owner")
	ApiBankSearchCmd.Flags().StringP("data-class", "d", "", "Search by data class")

	ApiBankCmd.AddCommand(ApiBankCreateCmd)
	ApiBankCmd.AddCommand(ApiBankListCmd)
	ApiBankCmd.AddCommand(ApiBankGetCmd)
	ApiBankCmd.AddCommand(ApiBankAddAPICmd)
	ApiBankCmd.AddCommand(ApiBankSearchCmd)
}
