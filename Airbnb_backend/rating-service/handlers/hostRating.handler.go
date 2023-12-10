package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"net/http"
	"rating-service/domain"
	"rating-service/services"
	"time"
)

type HostRatingHandler struct {
	hostRatingService services.HostRatingService
	DB                *mongo.Collection
}

func NewHostRatingHandler(hostRatingService services.HostRatingService, db *mongo.Collection) HostRatingHandler {
	return HostRatingHandler{hostRatingService, db}
}
func (s *HostRatingHandler) RateHost(c *gin.Context) {
	hostID := c.Param("hostId")

	token := c.GetHeader("Authorization")
	currentUser, err := s.getCurrentUserFromAuthService(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to obtain current user information"})
		return
	}

	hostUser, err := s.getUserByIDFromAuthService(hostID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to obtain host user information"})
		return
	}

	currentDateTime := primitive.NewDateTimeFromTime(time.Now())

	rating := domain.Rating("5")
	newRateHost := &domain.RateHost{
		Host:        hostUser,
		Guest:       currentUser,
		DateAndTime: currentDateTime,
		Rating:      rating,
	}

	err = s.hostRatingService.SaveRating(newRateHost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rating"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Rating successfully saved", "rating": newRateHost})
}

func (s *HostRatingHandler) getUserByIDFromAuthService(userID string) (*domain.User, error) {
	url := "https://auth-server:8080/api/users/getById/" + userID

	timeout := 2000 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := s.HTTPSPerformAuthorizationRequestWithContext(ctx, "", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("User not found")
	}

	var user domain.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *HostRatingHandler) getCurrentUserFromAuthService(token string) (*domain.User, error) {
	url := "https://auth-server:8080/api/users/currentUser"

	timeout := 2000 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := s.HTTPSPerformAuthorizationRequestWithContext(ctx, token, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Unauthorized")
	}
	var user domain.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *HostRatingHandler) HTTPSPerformAuthorizationRequestWithContext(ctx context.Context, token string, url string) (*http.Response, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return resp, nil
}