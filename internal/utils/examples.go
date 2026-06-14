package utils

// This file contains practical examples and integration patterns for using
// SQL Injection Protection and Input Validation in the AxiomNizam application

import (
	"fmt"
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"time"
)

// =============================================================================
// Example 1: Handler Integration Pattern
// =============================================================================

// Example of how to integrate validation in your API handlers
// This demonstrates the recommended pattern for all API endpoints

/*
package handlers

import (
	"github.com/gin-gonic/gin"
	"internal/utils"
)

type UserRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	var req UserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// Create validator and batch validate all fields
	validator := utils.NewInputValidator()
	batch := validator.NewValidationBatch().
		AddStringValidation("first_name", req.FirstName,
			utils.WithMinLength(2), utils.WithMaxLength(50)).
		AddStringValidation("last_name", req.LastName,
			utils.WithMinLength(2), utils.WithMaxLength(50)).
		AddEmailValidation("email", req.Email).
		AddStringValidation("username", req.Username,
			utils.WithMinLength(3), utils.WithMaxLength(20)).
		AddPasswordValidation("password", req.Password,
			utils.WithMinLength(12), utils.WithRequireSpecialChars(true))

	if batch.HasErrors() {
		c.JSON(400, gin.H{"validation_errors": batch.GetErrors()})
		return
	}

	// At this point, all input is validated and safe
	// Proceed with database operations
	user := models.User{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Username:  req.Username,
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(201, user)
}
*/

// =============================================================================
// Example 2: Dynamic Query Building Pattern
// =============================================================================

/*
func (h *UserHandler) SearchUsers(c *gin.Context) {
	// Get query parameters
	searchQuery := c.Query("q")
	sortBy := c.Query("sort")
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	// Validate and sanitize all parameters
	sqlProtection := utils.NewSQLInjectionProtection()

	// Validate search query
	if err := sqlProtection.ValidateSQLInput(searchQuery); err != nil {
		c.JSON(400, gin.H{"error": "Invalid search query"})
		return
	}

	// Sanitize for LIKE queries
	sanitizedSearch, _ := sqlProtection.SanitizeSearchInput(searchQuery)

	// Validate sort column
	orderBy, err := sqlProtection.SanitizeOrderBy(sortBy)
	if err != nil {
		orderBy = "id ASC" // Safe default
	}

	// Validate pagination
	page, _ := strconv.Atoi(pageStr)
	limitNum, _ := strconv.Atoi(limitStr)
	limit, offset, err := sqlProtection.SanitizeLimitOffset(limitNum, (page-1)*limitNum)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid pagination parameters"})
		return
	}

	// Execute protected query with parameterized values
	var users []models.User
	h.db.Where("name LIKE ?", "%"+sanitizedSearch+"%").
		Order(orderBy).
		Limit(limit).
		Offset(offset).
		Find(&users)

	c.JSON(200, gin.H{
		"data": users,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
		},
	})
}
*/

// =============================================================================
// Example 3: Database Query with Dynamic Columns
// =============================================================================

/*
func (h *DataHandler) GetDynamicReport(c *gin.Context) {
	tableName := c.Query("table")
	columns := c.QueryArray("columns")
	filter := c.Query("filter")

	sqlProtection := utils.NewSQLInjectionProtection()

	// Validate table name
	safeTable, err := sqlProtection.SanitizeTableName(tableName)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid table name"})
		return
	}

	// Validate all column names
	safeColumns := []string{}
	for _, col := range columns {
		safeCol, err := sqlProtection.SanitizeColumnName(col)
		if err != nil {
			c.JSON(400, gin.H{"error": fmt.Sprintf("Invalid column: %s", col)})
			return
		}
		safeColumns = append(safeColumns, safeCol)
	}

	// Validate filter - use parameterized query pattern
	if err := sqlProtection.ValidateSQLInput(filter); err != nil {
		c.JSON(400, gin.H{"error": "Invalid filter"})
		return
	}

	// Build safe query using GORM
	query := h.db.Table(safeTable).Select(safeColumns)

	// Only add WHERE if filter is provided
	if filter != "" {
		// Use parameterized queries with GORM
		query = query.Where(filter) // GORM handles parameterization
	}

	var results []map[string]interface{}
	if err := query.Find(&results).Error; err != nil {
		c.JSON(500, gin.H{"error": "Query failed"})
		return
	}

	c.JSON(200, results)
}
*/

// =============================================================================
// Example 4: File Upload Validation Pattern
// =============================================================================

/*
func (h *FileHandler) UploadDocument(c *gin.Context) {
	file, err := c.FormFile("document")
	if err != nil {
		c.JSON(400, gin.H{"error": "No file provided"})
		return
	}

	// Validate filename
	if err := utils.ValidateFileNameInput(file.Filename); err != nil {
		c.JSON(400, gin.H{"error": "Invalid filename"})
		return
	}

	// Validate file size (e.g., max 50MB)
	maxFileSize := int64(50 * 1024 * 1024)
	if file.Size > maxFileSize {
		c.JSON(400, gin.H{"error": "File too large"})
		return
	}

	// Generate safe filename
	safeFilename := fmt.Sprintf("%d_%s", time.Now().Unix(), file.Filename)

	// Validate path
	uploadPath := "uploads/documents"
	if err := utils.ValidatePath(uploadPath); err != nil {
		c.JSON(500, gin.H{"error": "Server configuration error"})
		return
	}

	// Save file
	fullPath := filepath.Join(uploadPath, safeFilename)
	if err := c.SaveUploadedFile(file, fullPath); err != nil {
		c.JSON(500, gin.H{"error": "Failed to save file"})
		return
	}

	// Store file metadata in database (with validation)
	fileRecord := models.UploadedFile{
		OriginalName: file.Filename,
		SavedName:    safeFilename,
		Size:         file.Size,
		Path:         fullPath,
	}

	if err := h.db.Create(&fileRecord).Error; err != nil {
		c.JSON(500, gin.H{"error": "Failed to save file record"})
		return
	}

	c.JSON(200, fileRecord)
}
*/

// =============================================================================
// Example 5: Authentication Input Validation
// =============================================================================

/*
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	validator := utils.NewInputValidator()

	// Validate email format
	if err := validator.ValidateEmail(req.Email); err != nil {
		// Don't reveal whether email exists or not
		c.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}

	// Validate password is not empty (full validation happens after user lookup)
	if IsEmpty(req.Password) {
		c.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}

	// Look up user by email
	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// Intentionally vague to prevent user enumeration
		c.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}

	// Verify password
	if !VerifyPassword(user.PasswordHash, req.Password) {
		c.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}

	// Generate JWT token
	token, err := GenerateToken(user.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(200, gin.H{
		"token": token,
		"user":  user,
	})
}
*/

// =============================================================================
// Example 6: Query Parameter Validation Middleware
// =============================================================================

/*
// ValidateQueryParams is a middleware that validates common query parameters
func ValidateQueryParams(c *gin.Context) {
	validator := utils.NewInputValidator()
	sqlProtection := utils.NewSQLInjectionProtection()

	// Validate pagination parameters if present
	if page := c.Query("page"); page != "" {
		if err := validator.ValidateInteger(page, utils.WithMinValue(1)); err != nil {
			c.JSON(400, gin.H{"error": "Invalid page parameter"})
			c.Abort()
			return
		}
	}

	if limit := c.Query("limit"); limit != "" {
		limitInt, err := strconv.Atoi(limit)
		if err != nil || limitInt < 1 || limitInt > 100 {
			c.JSON(400, gin.H{"error": "Invalid limit parameter (must be 1-100)"})
			c.Abort()
			return
		}
	}

	// Validate search query if present
	if search := c.Query("search"); search != "" {
		if err := sqlProtection.ValidateSQLInput(search); err != nil {
			c.JSON(400, gin.H{"error": "Invalid search query"})
			c.Abort()
			return
		}
	}

	// Validate sort parameter if present
	if sort := c.Query("sort"); sort != "" {
		if _, err := sqlProtection.SanitizeOrderBy(sort); err != nil {
			c.JSON(400, gin.H{"error": "Invalid sort parameter"})
			c.Abort()
			return
		}
	}

	c.Next()
}

// Usage in router setup:
// router.Use(ValidateQueryParams)
*/

// =============================================================================
// Example 7: Advanced Batch Validation with Custom Validators
// =============================================================================

/*
type RegistrationRequest struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Email        string `json:"email"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	ConfirmPass  string `json:"confirm_password"`
	PhoneNumber  string `json:"phone_number"`
	DateOfBirth  string `json:"date_of_birth"`
	TermsAccepted bool  `json:"terms_accepted"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	validator := utils.NewInputValidator()
	batch := validator.NewValidationBatch()

	// Add all field validations
	batch.
		AddStringValidation("first_name", req.FirstName,
			utils.WithMinLength(2), utils.WithMaxLength(50),
			utils.WithPattern(`^[a-zA-Z\s'-]+$`)).
		AddStringValidation("last_name", req.LastName,
			utils.WithMinLength(2), utils.WithMaxLength(50),
			utils.WithPattern(`^[a-zA-Z\s'-]+$`)).
		AddEmailValidation("email", req.Email).
		AddStringValidation("username", req.Username,
			utils.WithMinLength(3), utils.WithMaxLength(20)).
		AddPasswordValidation("password", req.Password,
			utils.WithMinLength(12), utils.WithRequireUppercase(true)).
		AddIntegerValidation("phone_number", req.PhoneNumber)

	if batch.HasErrors() {
		c.JSON(400, gin.H{"validation_errors": batch.GetErrors()})
		return
	}

	// Additional custom validations
	if req.Password != req.ConfirmPass {
		c.JSON(400, gin.H{"error": "Passwords do not match"})
		return
	}

	if !req.TermsAccepted {
		c.JSON(400, gin.H{"error": "You must accept the terms and conditions"})
		return
	}

	// Check if user already exists (using parameterized query)
	var existingUser models.User
	if err := h.db.Where("email = ? OR username = ?", req.Email, req.Username).
		First(&existingUser).Error; err == nil {
		c.JSON(400, gin.H{"error": "Email or username already exists"})
		return
	}

	// All validations passed - proceed with registration
	// Hash password, create user, send verification email, etc.

	c.JSON(200, gin.H{"message": "Registration successful"})
}
*/

// =============================================================================
// Example 8: Data Export with Dynamic Filters
// =============================================================================

/*
func (h *ReportHandler) ExportData(c *gin.Context) {
	tableName := c.Query("table")
	filterJSON := c.Query("filters")
	exportFormat := c.Query("format") // "csv" or "json"

	sqlProtection := utils.NewSQLInjectionProtection()
	validator := utils.NewInputValidator()

	// Validate table name
	safeTable, err := sqlProtection.SanitizeTableName(tableName)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid table"})
		return
	}

	// Validate export format
	if exportFormat != "csv" && exportFormat != "json" {
		c.JSON(400, gin.H{"error": "Invalid export format"})
		return
	}

	// Validate filters JSON
	if err := validator.ValidateJSON(filterJSON); err != nil {
		c.JSON(400, gin.H{"error": "Invalid filters JSON"})
		return
	}

	// Parse and validate filter structure
	var filters map[string]interface{}
	json.Unmarshal([]byte(filterJSON), &filters)

	// Validate each filter key is a valid column name
	if err := sqlProtection.ValidateColumnFilter(filters); err != nil {
		c.JSON(400, gin.H{"error": "Invalid filter columns"})
		return
	}

	// Execute query with validated parameters
	var data []map[string]interface{}
	query := h.db.Table(safeTable)

	// Build WHERE clause from validated filters
	for col, val := range filters {
		safeCol, _ := sqlProtection.SanitizeColumnName(col)
		query = query.Where(fmt.Sprintf("%s = ?", safeCol), val)
	}

	if err := query.Find(&data).Error; err != nil {
		c.JSON(500, gin.H{"error": "Query failed"})
		return
	}

	// Format and return data
	if exportFormat == "csv" {
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename=export.csv")
		// TODO: Convert data to CSV
	} else {
		c.JSON(200, data)
	}
}
*/

// =============================================================================
// Helper Functions (Logging & Error Handling)
// =============================================================================

// LogValidationError logs validation errors safely
func LogValidationError(fieldName string, err error) {
	logging.Z().Info(fmt.Sprintf("[VALIDATION] Field: %s, Error: %v\n", fieldName, err))
}

// LogSecurityEvent logs security-related events
func LogSecurityEvent(eventType string, details map[string]interface{}) {
	logging.Z().Info(fmt.Sprintf("[SECURITY] Event: %s, Details: %+v\n", eventType, details))
}

// LogSQLInjectionAttempt logs potential SQL injection attempts
func LogSQLInjectionAttempt(input string, source string) {
	logging.Z().Info(fmt.Sprintf("[SECURITY] Potential SQL Injection Attempt\n"))
	logging.Z().Info(fmt.Sprintf("  Source: %s\n", source))
	logging.Z().Info(fmt.Sprintf("  Input length: %d\n", len(input)))
	logging.Z().Info(fmt.Sprintf("  Timestamp: %v\n", time.Now()))
}

// =============================================================================
// Best Practices Summary
// =============================================================================

/*
SECURITY CHECKLIST:

1. INPUT VALIDATION
   ✓ Validate all user inputs immediately upon receipt
   ✓ Use batch validation for multiple fields
   ✓ Apply both format and length validations
   ✓ Whitelist valid characters, don't blacklist invalid ones

2. SQL INJECTION PREVENTION
   ✓ NEVER concatenate strings in SQL queries
   ✓ ALWAYS use parameterized queries with ?
   ✓ Validate table/column names with SanitizeIdentifier
   ✓ Use GORM or database/sql drivers for safety

3. QUERY SECURITY
   ✓ Sanitize LIMIT and OFFSET values
   ✓ Validate ORDER BY clauses
   ✓ Escape wildcard characters in LIKE queries
   ✓ Use maximum length constraints on all inputs

4. ERROR HANDLING
   ✓ Never expose database error details to users
   ✓ Use generic error messages for security issues
   ✓ Log security events for monitoring
   ✓ Don't reveal whether users/data exists (prevent enumeration)

5. XSS PREVENTION
   ✓ Sanitize HTML input with SanitizeHTMLInput
   ✓ Escape output in templates
   ✓ Remove dangerous event handlers
   ✓ Use Content-Security-Policy headers

6. FILE OPERATIONS
   ✓ Validate filenames to prevent directory traversal
   ✓ Validate file paths before operations
   ✓ Check file sizes
   ✓ Store files outside web root when possible

7. AUTHENTICATION
   ✓ Validate email format
   ✓ Enforce strong passwords
   ✓ Hash passwords with bcrypt or similar
   ✓ Use rate limiting on login attempts

8. MONITORING
   ✓ Log all validation failures
   ✓ Log security events
   ✓ Alert on suspicious patterns
   ✓ Review logs regularly
*/

// These are placeholder examples showing the recommended patterns.
// Remove or adapt these according to your actual implementation.
