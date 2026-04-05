package model

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type BookingStatus string

const (
	BookingStatusActive    BookingStatus = "active"
	BookingStatusCancelled BookingStatus = "cancelled"
)

type UserClaims struct {
	UserID string
	Role   Role
}

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TokenResponse struct {
	Token string `json:"token"`
}

type Room struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Capacity    *int    `json:"capacity"`
	CreatedAt   *string `json:"createdAt"`
}

type Schedule struct {
	ID         string `json:"id"`
	RoomID     string `json:"roomId"`
	DaysOfWeek []int  `json:"daysOfWeek"`
	StartTime  string `json:"startTime"`
	EndTime    string `json:"endTime"`
}

type Slot struct {
	ID     string `json:"id"`
	RoomID string `json:"roomId"`
	Start  string `json:"start"`
	End    string `json:"end"`
}

type Booking struct {
	ID             string  `json:"id"`
	SlotID         string  `json:"slotId"`
	UserID         string  `json:"userId"`
	Status         string  `json:"status"`
	ConferenceLink *string `json:"conferenceLink"`
	CreatedAt      *string `json:"createdAt"`
}

type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}
