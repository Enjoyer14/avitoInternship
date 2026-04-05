package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"avito/internal/httpapi"
	"avito/internal/store"
)

func TestE2E_CreateRoomScheduleBooking(t *testing.T) {
	srv, cleanup := setupServer(t)
	defer cleanup()

	adminToken := dummyLogin(t, srv, "admin")
	userToken := dummyLogin(t, srv, "user")

	roomID := createRoom(t, srv, adminToken, "Room 1")
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	day := dayNumber(tomorrow)
	createSchedule(t, srv, adminToken, roomID, []int{day}, "09:00", "11:00")

	slotID := firstAvailableSlot(t, srv, userToken, roomID, tomorrow.Format("2006-01-02"))
	bookingID := createBooking(t, srv, userToken, slotID)
	if bookingID == "" {
		t.Fatal("empty booking id")
	}
}

func TestE2E_CancelBookingIdempotent(t *testing.T) {
	srv, cleanup := setupServer(t)
	defer cleanup()

	adminToken := dummyLogin(t, srv, "admin")
	userToken := dummyLogin(t, srv, "user")

	roomID := createRoom(t, srv, adminToken, "Room 2")
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	day := dayNumber(tomorrow)
	createSchedule(t, srv, adminToken, roomID, []int{day}, "13:00", "14:00")
	slotID := firstAvailableSlot(t, srv, userToken, roomID, tomorrow.Format("2006-01-02"))
	bookingID := createBooking(t, srv, userToken, slotID)

	status1 := cancelBooking(t, srv, userToken, bookingID)
	if status1 != "cancelled" {
		t.Fatalf("expected cancelled, got %s", status1)
	}
	status2 := cancelBooking(t, srv, userToken, bookingID)
	if status2 != "cancelled" {
		t.Fatalf("expected cancelled on second cancel, got %s", status2)
	}
}

func setupServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	dsn := "postgres://avito:avito@localhost:5433/avito?sslmode=disable"

	s, err := store.New(dsn)
	if err != nil {
		t.Skipf("postgres is not available: %v", err)
	}

	if _, err := s.DB.Exec(`DROP TABLE IF EXISTS bookings, slots, schedules, rooms CASCADE`); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if err := s.RunMigrations("../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	h := &httpapi.Handler{Store: s, JWTSecret: "ochen-secretniy-kluch"}
	srv := httptest.NewServer(h.Router())

	cleanup := func() {
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = s.DB.ExecContext(ctx, `DROP TABLE IF EXISTS bookings, slots, schedules, rooms CASCADE`)
		_ = s.Close()
	}
	return srv, cleanup
}

func dummyLogin(t *testing.T, srv *httptest.Server, role string) string {
	resp := postJSON(t, srv, "", "/dummyLogin", map[string]any{"role": role}, http.StatusOK)
	return asString(resp, "token")
}

func createRoom(t *testing.T, srv *httptest.Server, token, name string) string {
	resp := postJSON(t, srv, token, "/rooms/create", map[string]any{"name": name}, http.StatusCreated)
	room, ok := resp["room"].(map[string]any)
	if !ok {
		t.Fatal("room object expected")
	}
	return asString(room, "id")
}

func createSchedule(t *testing.T, srv *httptest.Server, token, roomID string, days []int, start, end string) {
	payload := map[string]any{
		"roomId":     roomID,
		"daysOfWeek": days,
		"startTime":  start,
		"endTime":    end,
	}
	postJSON(t, srv, token, "/rooms/"+roomID+"/schedule/create", payload, http.StatusCreated)
}

func firstAvailableSlot(t *testing.T, srv *httptest.Server, token, roomID, date string) string {
	url := "/rooms/" + roomID + "/slots/list?date=" + date
	resp := getJSON(t, srv, token, url, http.StatusOK)
	slotsAny, ok := resp["slots"].([]any)
	if !ok || len(slotsAny) == 0 {
		t.Fatal("expected non-empty slots")
	}
	slot, ok := slotsAny[0].(map[string]any)
	if !ok {
		t.Fatal("slot object expected")
	}
	return asString(slot, "id")
}

func createBooking(t *testing.T, srv *httptest.Server, token, slotID string) string {
	resp := postJSON(t, srv, token, "/bookings/create", map[string]any{"slotId": slotID}, http.StatusCreated)
	booking, ok := resp["booking"].(map[string]any)
	if !ok {
		t.Fatal("booking object expected")
	}
	return asString(booking, "id")
}

func cancelBooking(t *testing.T, srv *httptest.Server, token, bookingID string) string {
	resp := postJSON(t, srv, token, "/bookings/"+bookingID+"/cancel", map[string]any{}, http.StatusOK)
	booking, ok := resp["booking"].(map[string]any)
	if !ok {
		t.Fatal("booking object expected")
	}
	return asString(booking, "status")
}

func dayNumber(date time.Time) int {
	if date.Weekday() == time.Sunday {
		return 7
	}
	return int(date.Weekday())
}

func postJSON(t *testing.T, srv *httptest.Server, token, path string, payload any, wantStatus int) map[string]any {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != wantStatus {
		t.Fatalf("unexpected status for %s: got=%d want=%d", path, res.StatusCode, wantStatus)
	}

	var decoded map[string]any
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

func getJSON(t *testing.T, srv *httptest.Server, token, path string, wantStatus int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http do: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != wantStatus {
		t.Fatalf("unexpected status for %s: got=%d want=%d", path, res.StatusCode, wantStatus)
	}
	var decoded map[string]any
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

func asString(m map[string]any, key string) string {
	v, _ := m[key]
	s, _ := v.(string)
	return s
}
