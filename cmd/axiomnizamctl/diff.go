package main

import (
	"context"
	"fmt"
	"os"

	"axiomnizam.bitbd.net/axiomnizam/internal/diff"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"
)

var DiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show what would change",
	Long:  "Display differences before applying a resource",
}

var DiffFileCmd = &cobra.Command{
	Use:   "diff -f resource.yaml",
	Short: "Diff a resource file",
	Long:  "Show what would change if you applied this resource",
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("filename")
		if filename == "" {
			return fmt.Errorf("--filename is required")
		}
		return handleDiffFile(filename)
	},
}

// handleDiffFile shows the diff for a file
func handleDiffFile(filename string) error {
	// Load the resource file
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var newResource map[string]interface{}
	if err := yaml.Unmarshal(data, &newResource); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Extract metadata
	kind := newResource["kind"].(string)
	metadata := newResource["metadata"].(map[string]interface{})
	name := metadata["name"].(string)

	// For demo, use empty old state (new resource)
	ctx := context.Background()
	diffResult, err := diff.Diff(ctx, kind, name, nil, newResource)
	if err != nil {
		return fmt.Errorf("failed to compute diff: %w", err)
	}

	fmt.Println(diff.PrintDiff(diffResult))

	return nil
}

func init() {
	DiffFileCmd.Flags().StringP("filename", "f", "", "Path to resource file")
	DiffCmd.AddCommand(DiffFileCmd)
}
