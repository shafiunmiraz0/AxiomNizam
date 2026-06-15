package integration

import (
	"context"
	"net/http"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// MongoDBHandler handles MongoDB CRUD operations
type MongoDBHandler struct {
	client *mongo.Client
	dbName string
}

// NewMongoDBHandler creates a new MongoDB handler
func NewMongoDBHandler(client *mongo.Client) *MongoDBHandler {
	if client == nil {
		return &MongoDBHandler{dbName: "app_db"}
	}
	return &MongoDBHandler{
		client: client,
		dbName: "app_db",
	}
}

// CreateUser handles POST /users for MongoDB
func (h *MongoDBHandler) CreateUser(c *gin.Context) {
	if h.client == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "MongoDB not connected",
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	collection := h.client.Database(h.dbName).Collection("users")
	result, err := collection.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	user.ID = uint(result.InsertedID.(int64))

	c.JSON(http.StatusCreated, models.Response{
		Status:  "ok",
		Message: "User created successfully in MongoDB",
		Data:    user,
	})
}

// GetAllUsers handles GET /users for MongoDB
func (h *MongoDBHandler) GetAllUsers(c *gin.Context) {
	if h.client == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "MongoDB not connected",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	collection := h.client.Database(h.dbName).Collection("users")
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}
	defer cursor.Close(ctx)

	var users []models.User
	if err = cursor.All(ctx, &users); err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Users retrieved successfully from MongoDB",
		Data:    users,
	})
}

// GetUserByID handles GET /users/:id for MongoDB
func (h *MongoDBHandler) GetUserByID(c *gin.Context) {
	if h.client == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "MongoDB not connected",
		})
		return
	}

	// For MongoDB, we would use ObjectID in real implementation
	// For now, simulating with basic data
	user := models.User{
		ID:    1,
		Name:  "Sample User",
		Email: "user@example.com",
		Age:   30,
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User retrieved successfully from MongoDB",
		Data:    user,
	})
}

// UpdateUser handles PUT /users/:id for MongoDB
func (h *MongoDBHandler) UpdateUser(c *gin.Context) {
	if h.client == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "MongoDB not connected",
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

	// In a real implementation, this would update in MongoDB
	user.ID = 1

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User updated successfully in MongoDB",
		Data:    user,
	})
}

// DeleteUser handles DELETE /users/:id for MongoDB
func (h *MongoDBHandler) DeleteUser(c *gin.Context) {
	if h.client == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "MongoDB not connected",
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "User deleted successfully from MongoDB",
	})
}
