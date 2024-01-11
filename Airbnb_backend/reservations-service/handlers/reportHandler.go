package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/gocql/gocql"
	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	ga "google.golang.org/api/analyticsdata/v1beta"
	"log"
	"net/http"
	"reservations-service/analyticsReport"
	"reservations-service/data"
	error2 "reservations-service/error"
	"reservations-service/repository"
	"strings"
	"time"
)

var validateFieldsReport = validator.New()

type KeyProductReport struct{}

type ReportHandler struct {
	logger    *log.Logger
	Repo      *repository.ReportRepo
	EventRepo *repository.EventRepo
	Tracer    trace.Tracer
}

func NewReportHandler(l *log.Logger, r *repository.ReportRepo, er *repository.EventRepo, tracer trace.Tracer) *ReportHandler {
	return &ReportHandler{l, r, er, tracer}
}

func (s *ReportHandler) GenerateDailyReportForAccommodation(rw http.ResponseWriter, h *http.Request) {
	ctx, span := s.Tracer.Start(h.Context(), "ReportHandler.GenerateDailyReportForAccommodation")
	defer span.End()

	token := h.Header.Get("Authorization")
	url := "https://auth-server:8080/api/users/currentUser"

	timeout := 2000 * time.Second // Adjust the timeout duration as needed
	ctxx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := s.HTTPSperformAuthorizationRequestWithContextReport(ctx, token, url)
	if err != nil {
		if ctxx.Err() == context.DeadlineExceeded {
			span.SetStatus(codes.Error, "Authorization service not available")
			errorMsg := map[string]string{"error": "Authorization service not available.."}
			error2.ReturnJSONError(rw, errorMsg, http.StatusInternalServerError)
			return
		}
		span.SetStatus(codes.Error, "Authorization service not available")
		errorMsg := map[string]string{"error": "Authorization service not available.."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	if statusCode != 200 {
		span.SetStatus(codes.Error, "Unauthorized")
		errorMsg := map[string]string{"error": "Unauthorized."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusUnauthorized)
		return
	}

	// Read the response body
	// Create a JSON decoder for the response body
	decoder := json.NewDecoder(resp.Body)

	// Define a struct to represent the JSON structure
	var response struct {
		LoggedInUser struct {
			ID       string        `json:"id"`
			UserRole data.UserRole `json:"userRole"`
		} `json:"user"`
		Message string `json:"message"`
	}

	// Decode the JSON response into the struct
	if err := decoder.Decode(&response); err != nil {
		if strings.Contains(err.Error(), "cannot parse") {
			span.SetStatus(codes.Error, "Invalid date format in the response")
			error2.ReturnJSONError(rw, "Invalid date format in the response", http.StatusBadRequest)
			return
		}
		span.SetStatus(codes.Error, "Error decoding JSON response")
		error2.ReturnJSONError(rw, fmt.Sprintf("Error decoding JSON response: %v", err), http.StatusBadRequest)
		return
	}

	if response.LoggedInUser.UserRole != data.Host {
		span.SetStatus(codes.Error, "Only hosts can see/generate reports!")
		error2.ReturnJSONError(rw, "Only hosts can see/generate reports!", http.StatusForbidden)
		return
	}

	vars := mux.Vars(h)
	accID := vars["accId"]

	urlAccommodationCheck := "https://acc-server:8083/api/accommodations/get/" + accID

	resp, err = s.HTTPSperformAuthorizationRequestWithContextReport(ctx, token, urlAccommodationCheck)
	if err != nil {
		if ctxx.Err() == context.DeadlineExceeded {
			span.SetStatus(codes.Error, "Accommodation service is not available")
			errorMsg := map[string]string{"error": "Accommodation service is not available."}
			error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
			return
		}
		span.SetStatus(codes.Error, "Accommodation service is not available")
		errorMsg := map[string]string{"error": "Accommodation service is not available."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()

	statusCodeAccommodation := resp.StatusCode
	fmt.Println(statusCodeAccommodation)
	if statusCodeAccommodation != 200 {
		span.SetStatus(codes.Error, "Accommodation with that id does not exist")
		errorMsg := map[string]string{"error": "Accommodation with that id does not exist."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
		return
	}

	var responseAccommodation struct {
		AccommodationName      string `json:"accommodation_name"`
		AccommodationLocation  string `json:"accommodation_location"`
		AccommodationHostId    string `json:"host_id"`
		AccommodationMinGuests int    `json:"accommodation_min_guests"`
		AccommodationMaxGuests int    `json:"accommodation_max_guests"`
	}
	decoder = json.NewDecoder(resp.Body)

	// Decode the JSON response into the struct
	if err := decoder.Decode(&responseAccommodation); err != nil {
		if strings.Contains(err.Error(), "cannot parse") {
			span.SetStatus(codes.Error, "Invalid date format.")
			errorMsg := map[string]string{"error": "Invalid date format."}
			error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
			return
		}
		span.SetStatus(codes.Error, "Error decoding JSON response:"+err.Error())
		error2.ReturnJSONError(rw, fmt.Sprintf("Error decoding JSON response: %v", err), http.StatusBadRequest)
		return
	}

	if responseAccommodation.AccommodationHostId != response.LoggedInUser.ID {
		span.SetStatus(codes.Error, "Unauthorized")
		errorMsg := map[string]string{"error": "Unauthorized: That is not your accommodation."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusUnauthorized)
		return
	}

	countReservations, err := s.EventRepo.CountReservationsForCurrentDay(ctx, accID)
	if err != nil {
		fmt.Println(err)
		fmt.Println("here")
		span.SetStatus(codes.Error, "Error counting reservations!")
		error2.ReturnJSONError(rw, "Error counting reservations!", http.StatusBadRequest)
		return
	}

	countRatings, err := s.EventRepo.CountRatingsForCurrentDay(ctx, accID)
	if err != nil {
		span.SetStatus(codes.Error, "Error counting ratings!")
		error2.ReturnJSONError(rw, "Error counting ratings!", http.StatusBadRequest)
		return
	}

	var Metrics = []*ga.Metric{
		{
			Name: "screenPageViews",
		},
		{
			Name: "averageSessionDuration",
		},
	}

	users, avgTime := analyticsReport.GetNumberOfUsersPerPage("yesterday", "today",
		"/accommodation/"+accID, Metrics)

	fmt.Println(users)
	fmt.Println("users report")
	fmt.Println(avgTime)
	fmt.Println("avg time report")

	reportID := gocql.TimeUUID()

	report := &data.DailyReport{
		ReportID:         data.TimeUUID(reportID),
		AccommodationID:  accID,
		Date:             time.Now(),
		ReservationCount: countReservations,
		RatingCount:      countRatings,
		AverageVisitTime: avgTime,
		PageVisits:       int(users),
	}

	errReport := s.Repo.InsertDailyReport(ctx, report)
	if errReport != nil {
		span.SetStatus(codes.Error, "Error storing report")
		s.logger.Print("Database exception: ", errReport)
		errorMsg := map[string]string{"error": "Error storing report"}
		error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
		return

	}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(rw).Encode(report); err != nil {
		s.logger.Printf("Error encoding response: %v", err)
	}
}

func (s *ReportHandler) GenerateMonthlyReportForAccommodation(rw http.ResponseWriter, h *http.Request) {
	ctx, span := s.Tracer.Start(h.Context(), "ReportHandler.GenerateMonthlyReportForAccommodation")
	defer span.End()

	token := h.Header.Get("Authorization")
	url := "https://auth-server:8080/api/users/currentUser"

	timeout := 2000 * time.Second // Adjust the timeout duration as needed
	ctxx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := s.HTTPSperformAuthorizationRequestWithContextReport(ctx, token, url)
	if err != nil {
		if ctxx.Err() == context.DeadlineExceeded {
			span.SetStatus(codes.Error, "Authorization service not available")
			errorMsg := map[string]string{"error": "Authorization service not available.."}
			error2.ReturnJSONError(rw, errorMsg, http.StatusInternalServerError)
			return
		}
		span.SetStatus(codes.Error, "Authorization service not available")
		errorMsg := map[string]string{"error": "Authorization service not available.."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	statusCode := resp.StatusCode
	if statusCode != 200 {
		span.SetStatus(codes.Error, "Unauthorized")
		errorMsg := map[string]string{"error": "Unauthorized."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusUnauthorized)
		return
	}

	// Read the response body
	// Create a JSON decoder for the response body
	decoder := json.NewDecoder(resp.Body)

	// Define a struct to represent the JSON structure
	var response struct {
		LoggedInUser struct {
			ID       string        `json:"id"`
			UserRole data.UserRole `json:"userRole"`
		} `json:"user"`
		Message string `json:"message"`
	}

	// Decode the JSON response into the struct
	if err := decoder.Decode(&response); err != nil {
		if strings.Contains(err.Error(), "cannot parse") {
			span.SetStatus(codes.Error, "Invalid date format in the response")
			error2.ReturnJSONError(rw, "Invalid date format in the response", http.StatusBadRequest)
			return
		}
		span.SetStatus(codes.Error, "Error decoding JSON response")
		error2.ReturnJSONError(rw, fmt.Sprintf("Error decoding JSON response: %v", err), http.StatusBadRequest)
		return
	}

	if response.LoggedInUser.UserRole != data.Host {
		span.SetStatus(codes.Error, "Only hosts can see/generate reports!")
		error2.ReturnJSONError(rw, "Only hosts can see/generate reports!", http.StatusForbidden)
		return
	}

	vars := mux.Vars(h)
	accID := vars["accId"]

	urlAccommodationCheck := "https://acc-server:8083/api/accommodations/get/" + accID

	resp, err = s.HTTPSperformAuthorizationRequestWithContextReport(ctx, token, urlAccommodationCheck)
	if err != nil {
		if ctxx.Err() == context.DeadlineExceeded {
			span.SetStatus(codes.Error, "Accommodation service is not available")
			errorMsg := map[string]string{"error": "Accommodation service is not available."}
			error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
			return
		}
		span.SetStatus(codes.Error, "Accommodation service is not available")
		errorMsg := map[string]string{"error": "Accommodation service is not available."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()

	statusCodeAccommodation := resp.StatusCode
	fmt.Println(statusCodeAccommodation)
	if statusCodeAccommodation != 200 {
		span.SetStatus(codes.Error, "Accommodation with that id does not exist")
		errorMsg := map[string]string{"error": "Accommodation with that id does not exist."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
		return
	}

	var responseAccommodation struct {
		AccommodationName      string `json:"accommodation_name"`
		AccommodationLocation  string `json:"accommodation_location"`
		AccommodationHostId    string `json:"host_id"`
		AccommodationMinGuests int    `json:"accommodation_min_guests"`
		AccommodationMaxGuests int    `json:"accommodation_max_guests"`
	}
	decoder = json.NewDecoder(resp.Body)

	// Decode the JSON response into the struct
	if err := decoder.Decode(&responseAccommodation); err != nil {
		if strings.Contains(err.Error(), "cannot parse") {
			span.SetStatus(codes.Error, "Invalid date format.")
			errorMsg := map[string]string{"error": "Invalid date format."}
			error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
			return
		}
		span.SetStatus(codes.Error, "Error decoding JSON response:"+err.Error())
		error2.ReturnJSONError(rw, fmt.Sprintf("Error decoding JSON response: %v", err), http.StatusBadRequest)
		return
	}

	if responseAccommodation.AccommodationHostId != response.LoggedInUser.ID {
		span.SetStatus(codes.Error, "Unauthorized")
		errorMsg := map[string]string{"error": "Unauthorized: That is not your accommodation."}
		error2.ReturnJSONError(rw, errorMsg, http.StatusUnauthorized)
		return
	}

	countReservations, err := s.EventRepo.CountReservationsForCurrentMonth(ctx, accID)
	if err != nil {
		span.SetStatus(codes.Error, "Error counting reservations!")
		error2.ReturnJSONError(rw, "Error counting reservations!", http.StatusBadRequest)
		return
	}

	countRatings, err := s.EventRepo.CountRatingsForCurrentMonth(ctx, accID)
	if err != nil {
		span.SetStatus(codes.Error, "Error counting ratings!")
		error2.ReturnJSONError(rw, "Error counting ratings!", http.StatusBadRequest)
		return
	}

	currentTime := time.Now().UTC()
	reportID := gocql.TimeUUID()
	currentYear, currentMonth, _ := currentTime.Date()

	currentDate := time.Now().UTC()
	startOfMonth := time.Date(currentDate.Year(), currentDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	startOfMonthStr := startOfMonth.Format("2006-01-02")
	endOfMonthStr := endOfMonth.Format("2006-01-02")

	var Metrics = []*ga.Metric{
		{
			Name: "screenPageViews",
		},
		{
			Name: "averageSessionDuration",
		},
	}

	users, avgTime := analyticsReport.GetNumberOfUsersPerPage(startOfMonthStr, endOfMonthStr,
		"/accommodation/"+accID, Metrics)

	fmt.Println(users)
	fmt.Println(avgTime)

	report := &data.MonthlyReport{
		ReportID:         data.TimeUUID(reportID),
		AccommodationID:  accID,
		ReservationCount: countReservations,
		RatingCount:      countRatings,
		Month:            int(currentMonth),
		Year:             currentYear,
		AverageVisitTime: avgTime,
		PageVisits:       int(users),
	}

	errReport := s.Repo.InsertMonthlyReport(ctx, report)
	if errReport != nil {
		fmt.Println("here error")
		fmt.Println(errReport)
		span.SetStatus(codes.Error, "Error storing report")
		s.logger.Print("Database exception: ", errReport)
		errorMsg := map[string]string{"error": "Error storing report"}
		error2.ReturnJSONError(rw, errorMsg, http.StatusBadRequest)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(rw).Encode(report); err != nil {
		s.logger.Printf("Error encoding response: %v", err)
	}

}

func (s *ReportHandler) HTTPSperformAuthorizationRequestWithContextReport(ctx context.Context, token string, url string) (*http.Response, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	// Perform the request with the provided context
	client := &http.Client{Transport: tr}
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return resp, nil
}
