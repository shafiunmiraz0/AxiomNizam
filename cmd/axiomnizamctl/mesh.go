package main

import (
	"context"
	"fmt"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/mesh"

	"github.com/spf13/cobra"
)

const (
	flagDomainName  = "Domain name"
	flagProductName = "Product name"
)

var MeshCmd = &cobra.Command{
	Use:   "mesh",
	Short: "Manage data mesh",
	Long:  "Create, list, and manage data mesh domains and products",
}

var MeshListCmd = &cobra.Command{
	Use:   "list",
	Short: "List data mesh instances",
	Long:  "List all data mesh domains and products",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleMeshList()
	},
}

var MeshStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check mesh status",
	Long:  "Check status of the data mesh",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleMeshStatus()
	},
}

var MeshDomainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage domains",
	Long:  "Manage data mesh domains",
}

var MeshDomainCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a domain",
	Long:  "Create a new data domain",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		owner, _ := cmd.Flags().GetString("owner")
		desc, _ := cmd.Flags().GetString("description")

		if name == "" {
			return fmt.Errorf("--name is required")
		}

		return handleCreateDomain(name, owner, desc)
	},
}

var MeshDomainListCmd = &cobra.Command{
	Use:   "list",
	Short: "List domains",
	Long:  "Display all data domains",
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleListDomains()
	},
}

var MeshDomainGetCmd = &cobra.Command{
	Use:   "get [domain-name]",
	Short: "Get domain details",
	Long:  "Display details of a data domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleGetDomain(args[0])
	},
}

var MeshProductCmd = &cobra.Command{
	Use:   "product",
	Short: "Manage data products",
	Long:  "Manage data products in domains",
}

var MeshProductCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a data product",
	Long:  "Create a new data product in a domain",
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		name, _ := cmd.Flags().GetString("name")
		owner, _ := cmd.Flags().GetString("owner")
		desc, _ := cmd.Flags().GetString("description")

		if domain == "" || name == "" {
			return fmt.Errorf("--domain and --name are required")
		}

		return handleCreateDataProduct(domain, name, owner, desc)
	},
}

var MeshProductListCmd = &cobra.Command{
	Use:   "list",
	Short: "List data products",
	Long:  "Display data products in a domain",
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		if domain == "" {
			return fmt.Errorf("--domain is required")
		}
		return handleListDataProducts(domain)
	},
}

var MeshProductGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get product details",
	Long:  "Display details of a data product",
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		name, _ := cmd.Flags().GetString("name")

		if domain == "" || name == "" {
			return fmt.Errorf("--domain and --name are required")
		}

		return handleGetDataProduct(domain, name)
	},
}

var MeshSubscribeCmd = &cobra.Command{
	Use:   "subscribe",
	Short: "Subscribe to a data product",
	Long:  "Subscribe to a data product port",
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		product, _ := cmd.Flags().GetString("product")
		subscriber, _ := cmd.Flags().GetString("subscriber")
		port, _ := cmd.Flags().GetString("port")

		if domain == "" || product == "" || subscriber == "" {
			return fmt.Errorf("--domain, --product, and --subscriber are required")
		}

		if port == "" {
			port = "default"
		}

		return handleSubscribeToProduct(domain, product, subscriber, port)
	},
}

var MeshLineageCmd = &cobra.Command{
	Use:   "lineage",
	Short: "View data lineage",
	Long:  "Trace data lineage through the mesh",
	RunE: func(cmd *cobra.Command, args []string) error {
		domain, _ := cmd.Flags().GetString("domain")
		product, _ := cmd.Flags().GetString("product")
		direction, _ := cmd.Flags().GetString("direction")

		if domain == "" || product == "" {
			return fmt.Errorf("--domain and --product are required")
		}

		return handleLineageTrace(domain, product, direction)
	},
}

// handleMeshList lists all data mesh domains and products
func handleMeshList() error {
	domains := mesh.ListDomains()

	fmt.Println("🌐 Data Mesh Overview")
	fmt.Println()
	fmt.Printf("Total Domains: %d\n\n", len(domains))

	fmt.Printf("%-25s %-20s %-10s %s\n", "DOMAIN", "OWNER", "PRODUCTS", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 80))

	totalProducts := 0
	for _, domain := range domains {
		totalProducts += len(domain.DataProducts)
		fmt.Printf("%-25s %-20s %-10d %s\n",
			domain.Name,
			domain.Owner,
			len(domain.DataProducts),
			domain.Description,
		)
	}

	fmt.Printf("\nTotal Products: %d\n", totalProducts)
	return nil
}

// handleMeshStatus shows mesh status
func handleMeshStatus() error {
	domains := mesh.ListDomains()

	fmt.Println("🌐 Data Mesh Status")
	fmt.Println()

	totalProducts := 0
	totalSubscriptions := 0

	for _, domain := range domains {
		totalProducts += len(domain.DataProducts)
		for _, product := range domain.DataProducts {
			totalSubscriptions += len(product.Subscriptions)
		}
	}

	fmt.Printf("Domains:       %d\n", len(domains))
	fmt.Printf("Products:      %d\n", totalProducts)
	fmt.Printf("Subscriptions: %d\n", totalSubscriptions)
	fmt.Println()

	if len(domains) == 0 {
		fmt.Println("ℹ️  No domains configured. Use 'axiomnizamctl mesh domain create' to get started.")
	} else {
		fmt.Println("✅ Mesh is operational")
	}

	return nil
}

// handleCreateDomain creates a domain
func handleCreateDomain(name, owner, description string) error {
	domain := &mesh.DataDomain{
		Name:         name,
		Owner:        owner,
		Description:  description,
		DataProducts: make([]mesh.DataProduct, 0),
		Labels:       make(map[string]string),
	}

	ctx := context.Background()
	if err := mesh.CreateDomain(ctx, domain); err != nil {
		return fmt.Errorf("failed to create domain: %w", err)
	}

	fmt.Printf("✅ Domain created: %s\n", name)
	fmt.Printf("   Owner: %s\n", owner)
	fmt.Printf("   Description: %s\n", description)

	return nil
}

// handleListDomains lists all domains
func handleListDomains() error {
	domains := mesh.ListDomains()

	fmt.Println("🌐 Data Mesh Domains")
	fmt.Println()
	fmt.Printf("%-25s %-20s %-10s %s\n", "NAME", "OWNER", "PRODUCTS", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 80))

	for _, domain := range domains {
		fmt.Printf("%-25s %-20s %-10d %s\n",
			domain.Name,
			domain.Owner,
			len(domain.DataProducts),
			domain.Description,
		)
	}

	return nil
}

// handleGetDomain gets details of a domain
func handleGetDomain(name string) error {
	domain := mesh.GetDomain(name)
	if domain == nil {
		return fmt.Errorf("domain not found: %s", name)
	}

	fmt.Printf("🌐 Domain: %s\n\n", name)
	fmt.Printf("Owner: %s\n", domain.Owner)
	fmt.Printf("Description: %s\n", domain.Description)
	fmt.Printf("Data Products: %d\n\n", len(domain.DataProducts))

	if len(domain.DataProducts) > 0 {
		fmt.Println("Products:")
		for i, product := range domain.DataProducts {
			fmt.Printf("  %d. %s\n", i+1, product.Name)
			fmt.Printf("     Owner: %s\n", product.Owner)
			fmt.Printf("     Subscriptions: %d\n", len(product.Subscriptions))
			if product.SLA.Availability != "" {
				fmt.Printf("     SLA Availability: %s\n", product.SLA.Availability)
			}
		}
	}

	return nil
}

// handleCreateDataProduct creates a data product
func handleCreateDataProduct(domainName, name, owner, description string) error {
	product := &mesh.DataProduct{
		Name:          name,
		Owner:         owner,
		Description:   description,
		SchemaVersion: "1.0.0",
		Schema:        make(map[string]interface{}),
		Ports:         make([]mesh.DataPort, 0),
		Subscriptions: make([]mesh.Subscription, 0),
		Tags:          make([]string, 0),
	}

	ctx := context.Background()
	if err := mesh.CreateDataProduct(ctx, domainName, product); err != nil {
		return fmt.Errorf("failed to create data product: %w", err)
	}

	fmt.Printf("✅ Data product created: %s\n", name)
	fmt.Printf("   Domain: %s\n", domainName)
	fmt.Printf("   Owner: %s\n", owner)

	return nil
}

// handleListDataProducts lists products in a domain
func handleListDataProducts(domainName string) error {
	domain := mesh.GetDomain(domainName)
	if domain == nil {
		return fmt.Errorf("domain not found: %s", domainName)
	}

	fmt.Printf("📊 Data Products in Domain: %s\n\n", domainName)
	fmt.Printf("%-25s %-20s %-15s %s\n", "NAME", "OWNER", "SUBSCRIPTIONS", "SLA")
	fmt.Println(strings.Repeat("─", 75))

	for _, product := range domain.DataProducts {
		sla := "None"
		if product.SLA.Availability != "" {
			sla = product.SLA.Availability
		}

		fmt.Printf("%-25s %-20s %-15d %s\n",
			product.Name,
			product.Owner,
			len(product.Subscriptions),
			sla,
		)
	}

	return nil
}

// handleGetDataProduct gets details of a data product
func handleGetDataProduct(domainName, productName string) error {
	return fmt.Errorf("data product lookup not implemented")
}

// handleSubscribeToProduct subscribes to a product
func handleSubscribeToProduct(domainName, productName, subscriberID, portName string) error {
	ctx := context.Background()
	subscription, err := mesh.Subscribe(ctx, domainName, productName, subscriberID, portName)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	fmt.Printf("✅ Subscription created: %s\n", subscription.ID)
	fmt.Printf("   Subscriber: %s\n", subscriberID)
	fmt.Printf("   Data Product: %s/%s\n", domainName, productName)
	fmt.Printf("   Port: %s\n", portName)
	fmt.Printf("   Status: %s\n", subscription.Status)

	return nil
}

// handleLineageTrace traces data lineage
func handleLineageTrace(domainName, productName, direction string) error {
	if direction == "" {
		direction = "downstream"
	}

	tracer := mesh.NewLineageTracer(mesh.GlobalDataMesh)

	fmt.Printf("📈 Data Lineage: %s/%s (%s)\n\n", domainName, productName, direction)

	if direction == "downstream" {
		subscriptions := tracer.TraceDownstream(domainName, productName)
		fmt.Printf("Downstream Consumers: %d\n", len(subscriptions))
		for _, sub := range subscriptions {
			fmt.Printf("  - %s (port: %s)\n", sub.SubscriberID, sub.PortName)
		}
	} else if direction == "upstream" {
		sources := tracer.TraceUpstream(domainName, productName)
		fmt.Printf("Upstream Sources: %d\n", len(sources))
		for _, source := range sources {
			fmt.Printf("  - %s\n", source)
		}
	}

	fmt.Println()
	related := tracer.DiscoverRelated(domainName, productName)
	fmt.Printf("Related Products: %d\n", len(related))
	for _, product := range related {
		fmt.Printf("  - %s/%s (owner: %s)\n", product.DomainName, product.Name, product.Owner)
	}

	return nil
}

func init() {
	MeshDomainCreateCmd.Flags().StringP("name", "n", "", flagDomainName)
	MeshDomainCreateCmd.Flags().StringP("owner", "o", "", "Domain owner")
	MeshDomainCreateCmd.Flags().StringP("description", "d", "", "Domain description")

	MeshProductCreateCmd.Flags().StringP("domain", "", "", flagDomainName)
	MeshProductCreateCmd.Flags().StringP("name", "n", "", flagProductName)
	MeshProductCreateCmd.Flags().StringP("owner", "o", "", "Product owner")
	MeshProductCreateCmd.Flags().StringP("description", "d", "", "Product description")

	MeshProductListCmd.Flags().StringP("domain", "", "", flagDomainName)
	MeshProductGetCmd.Flags().StringP("domain", "", "", flagDomainName)
	MeshProductGetCmd.Flags().StringP("name", "n", "", flagProductName)

	MeshSubscribeCmd.Flags().StringP("domain", "", "", flagDomainName)
	MeshSubscribeCmd.Flags().StringP("product", "p", "", flagProductName)
	MeshSubscribeCmd.Flags().StringP("subscriber", "s", "", "Subscriber ID")
	MeshSubscribeCmd.Flags().StringP("port", "", "default", "Output port name")

	MeshLineageCmd.Flags().StringP("domain", "", "", flagDomainName)
	MeshLineageCmd.Flags().StringP("product", "p", "", flagProductName)
	MeshLineageCmd.Flags().StringP("direction", "d", "downstream", "Direction (downstream/upstream)")

	MeshDomainCmd.AddCommand(MeshDomainCreateCmd)
	MeshDomainCmd.AddCommand(MeshDomainListCmd)
	MeshDomainCmd.AddCommand(MeshDomainGetCmd)

	MeshProductCmd.AddCommand(MeshProductCreateCmd)
	MeshProductCmd.AddCommand(MeshProductListCmd)
	MeshProductCmd.AddCommand(MeshProductGetCmd)

	MeshCmd.AddCommand(MeshListCmd)
	MeshCmd.AddCommand(MeshStatusCmd)
	MeshCmd.AddCommand(MeshDomainCmd)
	MeshCmd.AddCommand(MeshProductCmd)
	MeshCmd.AddCommand(MeshSubscribeCmd)
	MeshCmd.AddCommand(MeshLineageCmd)
}
