package transform

import (
	"net/http"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
	"github.com/gin-gonic/gin"
)

// Handler handles data transformation requests
type Handler struct {
	transformer *utils.DataTransformer
}

// NewHandler creates a new transformation handler
func NewHandler() *Handler {
	return &Handler{
		transformer: utils.NewDataTransformer(),
	}
}

// RegisterRule handles POST /api/transform/rules
func (th *Handler) RegisterRule(c *gin.Context) {
	var rule utils.TransformationRule

	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body: " + err.Error(),
		})
		return
	}

	if err := th.transformer.RegisterRule(&rule); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, models.Response{
		Status:  "ok",
		Message: "Transformation rule registered successfully",
		Data: map[string]interface{}{
			"name": rule.Name,
		},
	})
}

// GetRule handles GET /api/transform/rules/{name}
func (th *Handler) GetRule(c *gin.Context) {
	name := c.Param("name")

	rule, err := th.transformer.GetRule(name)
	if err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Transformation rule retrieved successfully",
		Data:    rule,
	})
}

// ListRules handles GET /api/transform/rules
func (th *Handler) ListRules(c *gin.Context) {
	rules := th.transformer.ListRules()

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Transformation rules retrieved successfully",
		Data: map[string]interface{}{
			"count": len(rules),
			"rules": rules,
		},
	})
}

// DeleteRule handles DELETE /api/transform/rules/{name}
func (th *Handler) DeleteRule(c *gin.Context) {
	name := c.Param("name")

	if err := th.transformer.DeleteRule(name); err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Transformation rule deleted successfully",
		Data: map[string]interface{}{
			"name": name,
		},
	})
}

// Transform handles POST /api/transform/apply
func (th *Handler) Transform(c *gin.Context) {
	var request struct {
		RuleName string      `json:"rule_name" binding:"required"`
		Data     interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body: " + err.Error(),
		})
		return
	}

	result, err := th.transformer.Transform(request.RuleName, request.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Data transformed successfully",
		Data:    result,
	})
}

// TransformBatch handles POST /api/transform/batch
func (th *Handler) TransformBatch(c *gin.Context) {
	var request struct {
		RuleName string        `json:"rule_name" binding:"required"`
		Data     []interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body: " + err.Error(),
		})
		return
	}

	results, err := th.transformer.TransformBatch(request.RuleName, request.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Batch data transformed successfully",
		Data: map[string]interface{}{
			"count":      len(results),
			"successful": countSuccessful(results),
			"failed":     len(results) - countSuccessful(results),
			"results":    results,
		},
	})
}

// PreviewTransformation handles POST /api/transform/preview
func (th *Handler) PreviewTransformation(c *gin.Context) {
	var request struct {
		RuleName string      `json:"rule_name" binding:"required"`
		Data     interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body: " + err.Error(),
		})
		return
	}

	result, err := th.transformer.Transform(request.RuleName, request.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Transformation preview generated",
		Data: map[string]interface{}{
			"original":    result.Original,
			"transformed": result.Transformed,
			"field_count": result.FieldCount,
			"duration":    result.Duration,
			"errors":      result.Errors,
		},
	})
}

// ExportRules handles GET /api/transform/rules/export
func (th *Handler) ExportRules(c *gin.Context) {
	data, err := th.transformer.ExportRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=\"transformation_rules.json\"")
	c.Data(http.StatusOK, "application/json", data)
}

// ImportRules handles POST /api/transform/rules/import
func (th *Handler) ImportRules(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "File is required",
		})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}
	defer src.Close()

	var buf [512]byte
	n, err := src.Read(buf[:])
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	if err := th.transformer.ImportRules(buf[:n]); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Failed to import rules: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Transformation rules imported successfully",
	})
}

// TestFieldRename handles POST /api/transform/test/rename
func (th *Handler) TestFieldRename(c *gin.Context) {
	var request struct {
		Data     map[string]interface{} `json:"data" binding:"required"`
		Mappings map[string]string      `json:"mappings" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	result := th.ApplyFieldMappings(request.Data, request.Mappings)

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Field renaming test completed",
		Data: map[string]interface{}{
			"original": request.Data,
			"renamed":  result,
			"mappings": request.Mappings,
		},
	})
}

// TestTypeConversion handles POST /api/transform/test/types
func (th *Handler) TestTypeConversion(c *gin.Context) {
	var request struct {
		Data        map[string]interface{} `json:"data" binding:"required"`
		Conversions map[string]string      `json:"conversions" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	result, errs := th.ApplyTypeConversions(request.Data, request.Conversions)

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Type conversion test completed",
		Data: map[string]interface{}{
			"original":    request.Data,
			"converted":   result,
			"conversions": request.Conversions,
			"errors":      errs,
		},
	})
}

// TestFlattening handles POST /api/transform/test/flatten
func (th *Handler) TestFlattening(c *gin.Context) {
	var request struct {
		Data   interface{}          `json:"data" binding:"required"`
		Config *utils.FlattenConfig `json:"config,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	if request.Config == nil {
		request.Config = &utils.FlattenConfig{
			Enabled:   true,
			Separator: ".",
		}
	}

	result, errs := th.ApplyFlattenJSON(request.Data, request.Config)

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "JSON flattening test completed",
		Data: map[string]interface{}{
			"original":  request.Data,
			"flattened": result,
			"config":    request.Config,
			"errors":    errs,
		},
	})
}

// Helper function
func countSuccessful(results []*utils.TransformedData) int {
	count := 0
	for _, result := range results {
		if len(result.Errors) == 0 {
			count++
		}
	}
	return count
}

// Make applyFieldMappings public
func (th *Handler) ApplyFieldMappings(data map[string]interface{}, mappings map[string]string) map[string]interface{} {
	return th.transformer.ApplyFieldMappings(data, mappings)
}

// Make applyTypeConversions public
func (th *Handler) ApplyTypeConversions(data map[string]interface{}, conversions map[string]string) (map[string]interface{}, []string) {
	return th.transformer.ApplyTypeConversions(data, conversions)
}

// Make flattenJSON public
func (th *Handler) ApplyFlattenJSON(data interface{}, config *utils.FlattenConfig) (map[string]interface{}, []string) {
	return th.transformer.ApplyFlattenJSON(data, config)
}

// Make flattenJSON public
func (th *Handler) FlattenJSON(data interface{}, config *utils.FlattenConfig) (map[string]interface{}, []string) {
	return th.transformer.ApplyFlattenJSON(data, config)
}
