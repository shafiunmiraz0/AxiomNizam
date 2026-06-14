package integration

import (
	"net/http"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OracleHandler handles Oracle Database CRUD operations
type OracleHandler struct {
	db *gorm.DB
}

// NewOracleHandler creates a new Oracle handler
func NewOracleHandler(db *gorm.DB) *OracleHandler {
	return &OracleHandler{db: db}
}

// CreateUser handles POST /users for Oracle
func (h *OracleHandler) CreateUser(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Oracle database not connected",
		})
		return
	}

	var user models.User
	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	if result := h.db.Create(&user); result.Error != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, models.Response{
		Status:  "ok",
		Message: "User created successfully in Oracle",
		Data:    user,
	})
}

// GetAllUsers handles GET /users for Oracle
func (h *OracleHandler) GetAllUsers(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Oracle database not connected",
		})
		return
	}

	var users []models.User
	if result := h.db.Find(&users); result.Error != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Users retrieved successfully from Oracle",
		Data:    users,
	})
}

// GetUserByID handles GET /users/:id for Oracle
func (h *OracleHandler) GetUserByID(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Oracle database not connected",
		})
		return
	}

	id := c.Param("id")
	var user models.User

	if result := h.db.First(&user, "id = ?", id); result.Error != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User retrieved successfully from Oracle",
		Data:    user,
	})
}

// UpdateUser handles PUT /users/:id for Oracle
func (h *OracleHandler) UpdateUser(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Oracle database not connected",
		})
		return
	}

	id := c.Param("id")
	var user models.User

	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	if result := h.db.Model(&models.User{}).Where("id = ?", id).Updates(&user); result.Error != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User updated successfully in Oracle",
		Data:    user,
	})
}

// DeleteUser handles DELETE /users/:id for Oracle
func (h *OracleHandler) DeleteUser(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Oracle database not connected",
		})
		return
	}

	id := c.Param("id")

	if result := h.db.Delete(&models.User{}, "id = ?", id); result.Error != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User deleted successfully from Oracle",
	})
}
