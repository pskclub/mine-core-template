package models

// Note is one note belonging to one user.
//
// The struct lives here rather than in modules/note because a model is a type,
// and types are shared: prisma migrates every table from one place, and
// coretest's AutoMigrate needs the whole list. What is *not* shared is the
// behaviour — which rows may be read, by whom, under what conditions. That lives
// in modules/note and nowhere else.
//
// UserID is a plain column, not a GORM association. Preloading the user here
// would let any package that can see this struct pull rows out of the users
// table, which belongs to modules/user.
type Note struct {
	BaseModel
	UserID string `json:"user_id" gorm:"column:user_id"`
	Title  string `json:"title" gorm:"column:title"`
	Body   string `json:"body" gorm:"column:body"`
	Pinned bool   `json:"pinned" gorm:"column:pinned"`
}

func (Note) TableName() string {
	return "notes"
}
