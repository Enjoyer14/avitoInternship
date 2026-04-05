package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"avito/internal/auth"
	"avito/internal/model"
	"avito/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type Handler struct {
	Store     *store.Store
	JWTSecret string
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/_info", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /dummyLogin", h.dummyLogin)
	mux.HandleFunc("GET /rooms/list", h.withAuth(h.listRooms, model.RoleAdmin, model.RoleUser))
	mux.HandleFunc("POST /rooms/create", h.withAuth(h.createRoom, model.RoleAdmin))
	mux.HandleFunc("POST /rooms/{roomId}/schedule/create", h.withAuth(h.createSchedule, model.RoleAdmin))
	mux.HandleFunc("GET /rooms/{roomId}/slots/list", h.withAuth(h.listSlots, model.RoleAdmin, model.RoleUser))
	mux.HandleFunc("POST /bookings/create", h.withAuth(h.createBooking, model.RoleUser))
	mux.HandleFunc("POST /bookings/{bookingId}/cancel", h.withAuth(h.cancelBooking, model.RoleUser))
	mux.HandleFunc("GET /bookings/my", h.withAuth(h.myBookings, model.RoleUser))
	mux.HandleFunc("GET /bookings/list", h.withAuth(h.listBookings, model.RoleAdmin))

	return mux
}

func (h *Handler) withAuth(next func(http.ResponseWriter, *http.Request, model.UserClaims), roles ...model.Role) http.HandlerFunc {
	allowed := make(map[model.Role]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			unauthorized(w)
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			unauthorized(w)
			return
		}

		claims, err := auth.ParseToken(h.JWTSecret, parts[1])
		if err != nil {
			unauthorized(w)
			return
		}

		if _, exists := allowed[claims.Role]; !exists {
			forbidden(w, "forbidden")
			return
		}

		next(w, r, claims)
	}
}

type dummyLoginRequest struct {
	Role string `json:"role"`
}

func (h *Handler) dummyLogin(w http.ResponseWriter, r *http.Request) {
	var req dummyLoginRequest
	if !readJSON(w, r, &req) {
		return
	}

	role := model.Role(strings.TrimSpace(req.Role))
	if role != model.RoleAdmin && role != model.RoleUser {
		invalidRequest(w, "role must be admin or user")
		return
	}

	token, err := auth.IssueToken(h.JWTSecret, role)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, model.TokenResponse{Token: token})
}

type createRoomRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Capacity    *int    `json:"capacity"`
}

func (h *Handler) createRoom(w http.ResponseWriter, r *http.Request, _ model.UserClaims) {
	var req createRoomRequest
	if !readJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		invalidRequest(w, "name is required")
		return
	}
	if req.Capacity != nil && *req.Capacity <= 0 {
		invalidRequest(w, "capacity must be positive")
		return
	}

	room, err := h.Store.CreateRoom(r.Context(), strings.TrimSpace(req.Name), req.Description, req.Capacity)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"room": room})
}

func (h *Handler) listRooms(w http.ResponseWriter, r *http.Request, _ model.UserClaims) {
	rooms, err := h.Store.ListRooms(r.Context())
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

type createScheduleRequest struct {
	RoomID     string `json:"roomId"`
	DaysOfWeek []int  `json:"daysOfWeek"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request, _ model.UserClaims) {
	roomID := r.PathValue("roomId")
	if !isValidUUID(roomID) {
		invalidRequest(w, "roomId must be valid UUID")
		return
	}

	var req createScheduleRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.RoomID == "" || req.RoomID != roomID {
		invalidRequest(w, "roomId in path and body must match")
		return
	}
	if !isValidUUID(req.RoomID) {
		invalidRequest(w, "roomId must be valid UUID")
		return
	}

	days, ok := normalizeDays(req.DaysOfWeek)
	if !ok {
		invalidRequest(w, "daysOfWeek must contain values in range 1..7")
		return
	}
	if !isValidTimeRange(req.StartTime, req.EndTime) {
		invalidRequest(w, "invalid startTime/endTime")
		return
	}

	exists, err := h.Store.RoomExists(r.Context(), roomID)
	if err != nil {
		internalError(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found")
		return
	}

	schedule, err := h.Store.CreateSchedule(r.Context(), roomID, days, req.StartTime, req.EndTime)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "SCHEDULE_EXISTS", "schedule for this room exists")
			return
		}
		internalError(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"schedule": schedule})
}

func (h *Handler) listSlots(w http.ResponseWriter, r *http.Request, _ model.UserClaims) {
	roomID := r.PathValue("roomId")
	if !isValidUUID(roomID) {
		invalidRequest(w, "roomId must be valid UUID")
		return
	}
	dateRaw := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateRaw == "" {
		invalidRequest(w, "date is required")
		return
	}

	date, err := time.ParseInLocation("2006-01-02", dateRaw, time.UTC)
	if err != nil {
		invalidRequest(w, "date must be in YYYY-MM-DD")
		return
	}

	exists, err := h.Store.RoomExists(r.Context(), roomID)
	if err != nil {
		internalError(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found")
		return
	}

	schedule, hasSchedule, err := h.Store.GetScheduleByRoom(r.Context(), roomID)
	if err != nil {
		internalError(w)
		return
	}
	if !hasSchedule {
		writeJSON(w, http.StatusOK, map[string]any{"slots": []model.Slot{}})
		return
	}

	weekday := dayOfWeek(date)
	if !containsDay(schedule.DaysOfWeek, weekday) {
		writeJSON(w, http.StatusOK, map[string]any{"slots": []model.Slot{}})
		return
	}

	intervals, ok := generateDailyIntervals(date, schedule.StartTime, schedule.EndTime)
	if !ok {
		internalError(w)
		return
	}
	for _, it := range intervals {
		if err := h.Store.UpsertSlot(r.Context(), roomID, it[0], it[1]); err != nil {
			internalError(w)
			return
		}
	}

	slots, err := h.Store.ListAvailableSlotsByDate(r.Context(), roomID, date)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slots": slots})
}

type createBookingRequest struct {
	SlotID               string `json:"slotId"`
	CreateConferenceLink bool   `json:"createConferenceLink"`
}

func (h *Handler) createBooking(w http.ResponseWriter, r *http.Request, claims model.UserClaims) {
	var req createBookingRequest
	if !readJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SlotID) == "" {
		invalidRequest(w, "slotId is required")
		return
	}
	if !isValidUUID(req.SlotID) {
		invalidRequest(w, "slotId must be valid UUID")
		return
	}

	booking, reason, err := h.Store.CreateBooking(r.Context(), req.SlotID, claims.UserID)
	if err != nil {
		internalError(w)
		return
	}
	switch reason {
	case "slot_not_found":
		writeError(w, http.StatusNotFound, "SLOT_NOT_FOUND", "slot not found")
		return
	case "slot_in_past":
		invalidRequest(w, "cannot create booking in the past")
		return
	case "slot_already_booked":
		writeError(w, http.StatusConflict, "SLOT_ALREADY_BOOKED", "slot is already booked")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"booking": booking})
}

func (h *Handler) cancelBooking(w http.ResponseWriter, r *http.Request, claims model.UserClaims) {
	bookingID := r.PathValue("bookingId")
	if !isValidUUID(bookingID) {
		invalidRequest(w, "bookingId must be valid UUID")
		return
	}

	bookingRow, exists, err := h.Store.GetBookingByID(r.Context(), bookingID)
	if err != nil {
		internalError(w)
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "BOOKING_NOT_FOUND", "booking not found")
		return
	}
	if bookingRow.UserID != claims.UserID {
		forbidden(w, "cannot cancel another user's booking")
		return
	}

	if bookingRow.Status == string(model.BookingStatusCancelled) {
		writeJSON(w, http.StatusOK, map[string]any{"booking": toBookingModel(bookingRow)})
		return
	}

	cancelled, err := h.Store.CancelBooking(r.Context(), bookingID)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"booking": toBookingModel(cancelled)})
}

func (h *Handler) myBookings(w http.ResponseWriter, r *http.Request, claims model.UserClaims) {
	bookings, err := h.Store.ListMyBookings(r.Context(), claims.UserID)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": bookings})
}

func (h *Handler) listBookings(w http.ResponseWriter, r *http.Request, _ model.UserClaims) {
	page, pageSize, ok := parsePagination(r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"))
	if !ok {
		invalidRequest(w, "invalid page or pageSize")
		return
	}

	bookings, total, err := h.Store.ListAllBookings(r.Context(), page, pageSize)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bookings":   bookings,
		"pagination": model.Pagination{Page: page, PageSize: pageSize, Total: total},
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		invalidRequest(w, "invalid request")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, model.ErrorBody{
		Error: model.ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

func unauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
}

func forbidden(w http.ResponseWriter, message string) {
	writeError(w, http.StatusForbidden, "FORBIDDEN", message)
}

func invalidRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "INVALID_REQUEST", message)
}

func internalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func parsePagination(pageRaw, pageSizeRaw string) (int, int, bool) {
	page := 1
	pageSize := 20
	if strings.TrimSpace(pageRaw) != "" {
		v, err := strconv.Atoi(pageRaw)
		if err != nil || v < 1 {
			return 0, 0, false
		}
		page = v
	}
	if strings.TrimSpace(pageSizeRaw) != "" {
		v, err := strconv.Atoi(pageSizeRaw)
		if err != nil || v < 1 || v > 100 {
			return 0, 0, false
		}
		pageSize = v
	}
	return page, pageSize, true
}

func normalizeDays(days []int) ([]int, bool) {
	if len(days) == 0 {
		return nil, false
	}
	seen := map[int]struct{}{}
	result := make([]int, 0, len(days))
	for _, day := range days {
		if day < 1 || day > 7 {
			return nil, false
		}
		if _, exists := seen[day]; exists {
			continue
		}
		seen[day] = struct{}{}
		result = append(result, day)
	}
	sort.Ints(result)
	return result, true
}

func isValidTimeRange(start, end string) bool {
	sHour, sMin, ok := parseHHMM(start)
	if !ok {
		return false
	}
	eHour, eMin, ok := parseHHMM(end)
	if !ok {
		return false
	}
	return (eHour*60 + eMin) > (sHour*60 + sMin)
}

func parseHHMM(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

func generateDailyIntervals(date time.Time, start, end string) ([][2]time.Time, bool) {
	sHour, sMin, ok := parseHHMM(start)
	if !ok {
		return nil, false
	}
	eHour, eMin, ok := parseHHMM(end)
	if !ok {
		return nil, false
	}

	startDateTime := time.Date(date.Year(), date.Month(), date.Day(), sHour, sMin, 0, 0, time.UTC)
	endDateTime := time.Date(date.Year(), date.Month(), date.Day(), eHour, eMin, 0, 0, time.UTC)
	if !endDateTime.After(startDateTime) {
		return nil, false
	}

	intervals := make([][2]time.Time, 0)
	for current := startDateTime; current.Before(endDateTime); current = current.Add(30 * time.Minute) {
		next := current.Add(30 * time.Minute)
		if next.After(endDateTime) {
			break
		}
		intervals = append(intervals, [2]time.Time{current, next})
	}
	return intervals, true
}

func dayOfWeek(date time.Time) int {
	wd := date.Weekday()
	if wd == time.Sunday {
		return 7
	}
	return int(wd)
}

func containsDay(days []int, day int) bool {
	for _, v := range days {
		if v == day {
			return true
		}
	}
	return false
}

func toBookingModel(row store.BookingRow) model.Booking {
	createdAt := row.CreatedAt.UTC().Format(time.RFC3339)
	return model.Booking{
		ID:             row.ID,
		SlotID:         row.SlotID,
		UserID:         row.UserID,
		Status:         row.Status,
		ConferenceLink: row.ConferenceLink,
		CreatedAt:      &createdAt,
	}
}

func isValidUUID(value string) bool {
	_, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil
}
