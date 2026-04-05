CREATE TABLE IF NOT EXISTS rooms (
    room_id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NULL,
    capacity INTEGER NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS schedules (
    schedule_id UUID PRIMARY KEY,
    room_id UUID NOT NULL UNIQUE REFERENCES rooms(room_id) ON DELETE CASCADE,
    days_of_week TEXT NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS slots (
    slot_id UUID PRIMARY KEY,
    room_id UUID NOT NULL REFERENCES rooms(room_id) ON DELETE CASCADE,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (room_id, start_at, end_at)
);

CREATE TABLE IF NOT EXISTS bookings (
    booking_id UUID PRIMARY KEY,
    slot_id UUID NOT NULL REFERENCES slots(slot_id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'cancelled')),
    conference_link TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_booking_per_slot
    ON bookings(slot_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_slots_room_start
    ON slots(room_id, start_at);

CREATE INDEX IF NOT EXISTS idx_bookings_user
    ON bookings(user_id);

CREATE INDEX IF NOT EXISTS idx_bookings_created
    ON bookings(created_at DESC);
