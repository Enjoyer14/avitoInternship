package store

import (
	"context"
	"testing"
	"time"
)

func TestStoreFlow(t *testing.T) {

	dbdns := "postgres://avito:avito@localhost:5433/avito?sslmode=disable"

	s, err := New(dbdns)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	defer s.Close()

	if _, err := s.DB.Exec(`DROP TABLE IF EXISTS bookings, slots, schedules, rooms CASCADE`); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if err := s.RunMigrations("../../migrations"); err != nil {
		t.Fatalf("migration: %v", err)
	}

	ctx := context.Background()

	room, err := s.CreateRoom(ctx, "Test Room", nil, nil)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	exists, err := s.RoomExists(ctx, room.ID)
	if err != nil || !exists {
		t.Fatalf("room exists error: %v exists=%v", err, exists)
	}

	rooms, err := s.ListRooms(ctx)
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("expected 1 room, got %d", len(rooms))
	}

	_, err = s.CreateSchedule(ctx, room.ID, []int{1, 2, 3}, "09:00", "10:00")
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	_, hasSchedule, err := s.GetScheduleByRoom(ctx, room.ID)
	if err != nil || !hasSchedule {
		t.Fatalf("get schedule: err=%v has=%v", err, hasSchedule)
	}

	futureStart := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	futureEnd := futureStart.Add(30 * time.Minute)
	pastStart := time.Now().UTC().Add(-26 * time.Hour).Truncate(time.Minute)
	pastEnd := pastStart.Add(30 * time.Minute)

	if err := s.UpsertSlot(ctx, room.ID, futureStart, futureEnd); err != nil {
		t.Fatalf("upsert future slot: %v", err)
	}
	if err := s.UpsertSlot(ctx, room.ID, pastStart, pastEnd); err != nil {
		t.Fatalf("upsert past slot: %v", err)
	}

	futureSlotID := getSlotID(t, s, room.ID, futureStart)
	pastSlotID := getSlotID(t, s, room.ID, pastStart)

	booking, reason, err := s.CreateBooking(ctx, futureSlotID, "22222222-2222-2222-2222-222222222222")
	if err != nil || reason != "" {
		t.Fatalf("create booking: err=%v reason=%s", err, reason)
	}
	if booking.Status != "active" {
		t.Fatalf("expected active booking, got %s", booking.Status)
	}

	available, err := s.ListAvailableSlotsByDate(ctx, room.ID, futureStart.Truncate(24*time.Hour))
	if err != nil {
		t.Fatalf("list available slots before cancel: %v", err)
	}
	if len(available) != 0 {
		t.Fatalf("expected 0 available slot with active booking but got %d", len(available))
	}

	_, reason, err = s.CreateBooking(ctx, futureSlotID, "22222222-2222-2222-2222-222222222222")
	if err != nil || reason != "slot_already_booked" {
		t.Fatalf("expected slot_already_booked, err=%v reason=%s", err, reason)
	}

	_, reason, err = s.CreateBooking(ctx, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "22222222-2222-2222-2222-222222222222")
	if err != nil || reason != "slot_not_found" {
		t.Fatalf("expected slot_not_found, err=%v reason=%s", err, reason)
	}

	_, reason, err = s.CreateBooking(ctx, pastSlotID, "22222222-2222-2222-2222-222222222222")
	if err != nil || reason != "slot_in_past" {
		t.Fatalf("expected slot_in_past, err=%v reason=%s", err, reason)
	}

	row, exists, err := s.GetBookingByID(ctx, booking.ID)
	if err != nil || !exists {
		t.Fatalf("get booking by id: err=%v exists=%v", err, exists)
	}
	if row.Status != "active" {
		t.Fatalf("unexpected status: %s", row.Status)
	}

	cancelled, err := s.CancelBooking(ctx, booking.ID)
	if err != nil {
		t.Fatalf("cancel booking: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("unexpected cancelled status: %s", cancelled.Status)
	}

	myBookings, err := s.ListMyBookings(ctx, "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("my bookings: %v", err)
	}
	if len(myBookings) != 1 {
		t.Fatalf("expected 1 my booking, got %d", len(myBookings))
	}

	all, total, err := s.ListAllBookings(ctx, 1, 20)
	if err != nil {
		t.Fatalf("list all bookings: %v", err)
	}
	if total != 1 || len(all) != 1 {
		t.Fatalf("unexpected all bookings: total=%d len=%d", total, len(all))
	}

	available, err = s.ListAvailableSlotsByDate(ctx, room.ID, futureStart.Truncate(24*time.Hour))
	if err != nil {
		t.Fatalf("list available slots after cancel: %v", err)
	}
	if len(available) != 1 {
		t.Fatalf("expected 1 available slot after cancellation, got %d", len(available))
	}
}

func getSlotID(t *testing.T, s *Store, roomID string, startAt time.Time) string {
	t.Helper()
	var slotID string
	err := s.DB.QueryRow(`SELECT slot_id FROM slots WHERE room_id=$1 AND start_at=$2`, roomID, startAt.UTC()).Scan(&slotID)
	if err != nil {
		t.Fatalf("query slot id: %v", err)
	}
	return slotID
}
