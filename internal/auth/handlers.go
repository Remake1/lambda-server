package auth

import (
	"lambda_server/internal/config"
	"lambda_server/internal/database"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type RegisterPayload struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type LoginPayload struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshTokenPayload struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// CustomClaims extends RegisteredClaims with token type
type CustomClaims struct {
	Type string `json:"type"`
	jwt.RegisteredClaims
}

// JwtKey holds the JWT secret key for signing tokens
var JwtKey []byte

// Init initializes the auth package with configuration
func Init(cfg *config.Config) {
	JwtKey = []byte(cfg.JWTSecret)
}

// Register godoc
// @Summary      Register a new user
// @Description  Register a new user with username, email, and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      RegisterPayload  true  "Registration payload"
// @Success      201      {object}  map[string]interface{}  "User registered successfully"
// @Failure      400      {object}  map[string]string  "Bad request"
// @Failure      500      {object}  map[string]string  "Internal server error"
// @Router       /auth/register [post]
func Register(c *gin.Context) {
	var payload RegisterPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	// Create the user
	newUser := database.User{
		Username: payload.Username,
		Email:    payload.Email,
		Password: string(hashedPassword),
	}

	result := database.DB.Create(&newUser)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully", "user_id": newUser.ID})
}

// Login godoc
// @Summary      Login user
// @Description  Authenticate user with email and password, returns access and refresh tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      LoginPayload  true  "Login payload"
// @Success      200      {object}  map[string]string  "Login successful"
// @Failure      400      {object}  map[string]string  "Bad request"
// @Failure      401      {object}  map[string]string  "Invalid credentials"
// @Failure      500      {object}  map[string]string  "Internal server error"
// @Router       /auth/login [post]
func Login(c *gin.Context) {
	var payload LoginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var user database.User
	result := database.DB.Where("email =?", payload.Email).First(&user)
	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Compare the provided password with the stored hash
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(payload.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Generate Access Token (15 minutes)
	accessExpirationTime := time.Now().Add(15 * time.Minute)
	accessClaims := &CustomClaims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(accessExpirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(JwtKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate access token"})
		return
	}

	// Generate Refresh Token (2 days)
	refreshExpirationTime := time.Now().Add(2 * 24 * time.Hour)
	refreshClaims := &CustomClaims{
		Type: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			ExpiresAt: jwt.NewNumericDate(refreshExpirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(JwtKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessTokenString,
		"refresh_token": refreshTokenString,
	})
}

// RefreshToken godoc
// @Summary      Refresh access token
// @Description  Generate a new access token using a valid refresh token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        payload  body      RefreshTokenPayload  true  "Refresh token payload"
// @Success      200      {object}  map[string]string  "New access token generated"
// @Failure      400      {object}  map[string]string  "Bad request"
// @Failure      401      {object}  map[string]string  "Invalid refresh token"
// @Failure      500      {object}  map[string]string  "Internal server error"
// @Router       /auth/refresh [post]
func RefreshToken(c *gin.Context) {
	var payload RefreshTokenPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse and validate the refresh token
	claims := &CustomClaims{}
	token, err := jwt.ParseWithClaims(payload.RefreshToken, claims, func(token *jwt.Token) (interface{}, error) {
		return JwtKey, nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// Verify token type is "refresh"
	if claims.Type != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token type"})
		return
	}

	// Generate new Access Token (15 minutes)
	accessExpirationTime := time.Now().Add(15 * time.Minute)
	accessClaims := &CustomClaims{
		Type: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.Subject,
			ExpiresAt: jwt.NewNumericDate(accessExpirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(JwtKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate access token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessTokenString,
	})
}
