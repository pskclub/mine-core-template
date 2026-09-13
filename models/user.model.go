package models

type User struct {
	BaseModel
	Email    string `json:"email" gorm:"column:email"`
	FullName string `json:"full_name" gorm:"column:full_name"`

	// Password is the bcrypt hash, never the plaintext. `json:"-"` keeps it out
	// of every response: the model is returned directly by the controllers, so
	// anything without that tag is public.
	Password string `json:"-" gorm:"column:password"`
}

func (User) TableName() string {
	return "users"
}
