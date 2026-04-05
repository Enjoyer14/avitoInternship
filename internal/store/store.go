package store

import (
	"avito/internal/model"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	DB *sql.DB
}

func New(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	return &Store{DB: db}, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) RunMigrations(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".sql" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)

	for _, path := range files {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if _, execErr := s.DB.Exec(string(content)); execErr != nil {
			return fmt.Errorf("migration %s: %w", path, execErr)
		}
	}
	return nil
}

type ScheduleRow struct {
	ID         string
	RoomID     string
	DaysOfWeek []int
	StartTime  string
	EndTime    string
}

type SlotRow struct {
	ID     string
	RoomID string
	Start  time.Time
	End    time.Time
}

type BookingRow struct {
	ID             string
	SlotID         string
	UserID         string
	Status         string
	ConferenceLink *string
	CreatedAt      time.Time
	SlotStart      *time.Time
}

func (s *Store) RoomExists(ctx context.Context, roomID string) (bool, error) {
	var exists bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM rooms WHERE room_id=$1)`, roomID).Scan(&exists)
	return exists, err
}

func (s *Store) CreateRoom(ctx context.Context, name string, description *string, capacity *int) (model.Room, error) {
	id := uuid.NewString()
	var createdAt time.Time
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO rooms (room_id, name, description, capacity) VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`, id, name, description, capacity).Scan(&createdAt)
	if err != nil {
		return model.Room{}, err
	}

	createdAtString := createdAt.UTC().Format(time.RFC3339)
	return model.Room{
		ID:          id,
		Name:        name,
		Description: description,
		Capacity:    capacity,
		CreatedAt:   &createdAtString,
	}, nil
}

func (s *Store) ListRooms(ctx context.Context) ([]model.Room, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT room_id, name, description, capacity, created_at FROM rooms ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rooms := make([]model.Room, 0)
	for rows.Next() {
		var room model.Room
		var createdAt time.Time
		if err := rows.Scan(&room.ID, &room.Name, &room.Description, &room.Capacity, &createdAt); err != nil {
			return nil, err
		}
		createdAtString := createdAt.UTC().Format(time.RFC3339)
		room.CreatedAt = &createdAtString
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
}

func (s *Store) CreateSchedule(ctx context.Context, roomID string, days []int, startTime, endTime string) (model.Schedule, error) {
	id := uuid.NewString()
	daysRaw := encodeDays(days)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO schedules (schedule_id, room_id, days_of_week, start_time, end_time) VALUES ($1, $2, $3, $4, $5)
	`, id, roomID, daysRaw, startTime, endTime)
	if err != nil {
		return model.Schedule{}, err
	}
	return model.Schedule{ID: id, RoomID: roomID, DaysOfWeek: days, StartTime: startTime, EndTime: endTime}, nil
}

func (s *Store) GetScheduleByRoom(ctx context.Context, roomID string) (ScheduleRow, bool, error) {
	var row ScheduleRow
	var daysRaw string
	err := s.DB.QueryRowContext(ctx, `
		SELECT schedule_id, room_id, days_of_week, start_time, end_time FROM schedules WHERE room_id=$1
	`, roomID).Scan(&row.ID, &row.RoomID, &daysRaw, &row.StartTime, &row.EndTime)
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduleRow{}, false, nil
	}
	if err != nil {
		return ScheduleRow{}, false, err
	}
	row.DaysOfWeek = decodeDays(daysRaw)
	return row, true, nil
}

func encodeDays(days []int) string {
	parts := make([]string, 0, len(days))
	for _, day := range days {
		parts = append(parts, strconv.Itoa(day))
	}
	return strings.Join(parts, ",")
}

func decodeDays(value string) []int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		result = append(result, v)
	}
	return result
}

func (s *Store) UpsertSlot(ctx context.Context, roomID string, start, end time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO slots (slot_id, room_id, start_at, end_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (room_id, start_at, end_at) DO NOTHING
	`, uuid.NewString(), roomID, start.UTC(), end.UTC())
	return err
}

func (s *Store) ListAvailableSlotsByDate(ctx context.Context, roomID string, date time.Time) ([]model.Slot, error) {
	dayStart := date.UTC()
	dayEnd := dayStart.Add(24 * time.Hour)

	rows, err := s.DB.QueryContext(ctx, `
		SELECT s.slot_id, s.room_id, s.start_at, s.end_at FROM slots s
		LEFT JOIN bookings b ON b.slot_id = s.slot_id AND b.status = 'active'
		WHERE s.room_id = $1 AND s.start_at >= $2 AND s.start_at < $3 AND b.booking_id IS NULL
		ORDER BY s.start_at ASC
	`, roomID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	slots := make([]model.Slot, 0)
	for rows.Next() {
		var slot model.Slot
		var startAt, endAt time.Time
		if err := rows.Scan(&slot.ID, &slot.RoomID, &startAt, &endAt); err != nil {
			return nil, err
		}
		slot.Start = startAt.UTC().Format(time.RFC3339)
		slot.End = endAt.UTC().Format(time.RFC3339)
		slots = append(slots, slot)
	}
	return slots, rows.Err()
}

func (s *Store) CreateBooking(ctx context.Context, slotID, userID string) (model.Booking, string, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return model.Booking{}, "", err
	}
	defer tx.Rollback()

	var startAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT start_at FROM slots WHERE slot_id=$1 FOR UPDATE`, slotID).Scan(&startAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Booking{}, "slot_not_found", nil
	}
	if err != nil {
		return model.Booking{}, "", err
	}
	if startAt.UTC().Before(time.Now().UTC()) {
		return model.Booking{}, "slot_in_past", nil
	}

	id := uuid.NewString()
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO bookings (booking_id, slot_id, user_id, status) VALUES ($1, $2, $3, 'active')
		RETURNING created_at
	`, id, slotID, userID).Scan(&createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.Booking{}, "slot_already_booked", nil
		}
		return model.Booking{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return model.Booking{}, "", err
	}

	createdAtString := createdAt.UTC().Format(time.RFC3339)
	booking := model.Booking{
		ID:        id,
		SlotID:    slotID,
		UserID:    userID,
		Status:    string(model.BookingStatusActive),
		CreatedAt: &createdAtString,
	}
	return booking, "", nil
}

func (s *Store) GetBookingByID(ctx context.Context, bookingID string) (BookingRow, bool, error) {
	var row BookingRow
	err := s.DB.QueryRowContext(ctx, `
		SELECT b.booking_id, b.slot_id, b.user_id, b.status, b.conference_link, b.created_at FROM bookings b
		WHERE b.booking_id = $1
	`, bookingID).Scan(&row.ID, &row.SlotID, &row.UserID, &row.Status, &row.ConferenceLink, &row.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BookingRow{}, false, nil
	}
	if err != nil {
		return BookingRow{}, false, err
	}
	return row, true, nil
}

func (s *Store) CancelBooking(ctx context.Context, bookingID string) (BookingRow, error) {
	var row BookingRow
	err := s.DB.QueryRowContext(ctx, `
		UPDATE bookings SET status = 'cancelled' WHERE booking_id = $1
		RETURNING booking_id, slot_id, user_id, status, conference_link, created_at
	`, bookingID).Scan(&row.ID, &row.SlotID, &row.UserID, &row.Status, &row.ConferenceLink, &row.CreatedAt)
	return row, err
}

func (s *Store) ListMyBookings(ctx context.Context, userID string) ([]model.Booking, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT b.booking_id, b.slot_id, b.user_id, b.status, b.conference_link, b.created_at FROM bookings b
		JOIN slots s ON s.slot_id = b.slot_id 
		WHERE b.user_id = $1 AND s.start_at >= NOW()
		ORDER BY s.start_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bookings := make([]model.Booking, 0)
	for rows.Next() {
		var booking model.Booking
		var createdAt time.Time
		if err := rows.Scan(&booking.ID, &booking.SlotID, &booking.UserID, &booking.Status, &booking.ConferenceLink, &createdAt); err != nil {
			return nil, err
		}
		createdAtString := createdAt.UTC().Format(time.RFC3339)
		booking.CreatedAt = &createdAtString
		bookings = append(bookings, booking)
	}
	return bookings, rows.Err()
}

func (s *Store) ListAllBookings(ctx context.Context, page, pageSize int) ([]model.Booking, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookings`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.DB.QueryContext(ctx, `
		SELECT b.booking_id, b.slot_id, b.user_id, b.status, b.conference_link, b.created_at FROM bookings b
		ORDER BY b.created_at DESC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	bookings := make([]model.Booking, 0)
	for rows.Next() {
		var booking model.Booking
		var createdAt time.Time
		if err := rows.Scan(&booking.ID, &booking.SlotID, &booking.UserID, &booking.Status, &booking.ConferenceLink, &createdAt); err != nil {
			return nil, 0, err
		}
		createdAtString := createdAt.UTC().Format(time.RFC3339)
		booking.CreatedAt = &createdAtString
		bookings = append(bookings, booking)
	}
	return bookings, total, rows.Err()
}
