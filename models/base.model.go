// Package models holds the shape of every table: structs and their column tags,
// and nothing else.
//
// It is the shared kernel — the one package every module may read from — which
// is why it holds no behaviour. Which rows may be read, by whom, under what
// conditions is a decision, and a decision belongs to the module that owns the
// table. A method here would be a decision every module could reach around.
//
// It imports nothing from this project, and arch enforces that.
package models

import (
	"time"

	"github.com/pskclub/mine-core/v2/utils"
	"gorm.io/gorm"
)

type BaseModel struct {
	ID        string         `json:"id" gorm:"column:id;primary_key"`
	CreatedAt *time.Time     `json:"created_at" gorm:"column:created_at"`
	UpdatedAt *time.Time     `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deleted_at"`
}

func NewBaseModel() BaseModel {
	return BaseModel{
		ID:        utils.NewUUID(),
		CreatedAt: utils.NowPtr(),
		UpdatedAt: utils.NowPtr(),
	}
}

type BaseModelHardDelete struct {
	ID        string     `json:"id" gorm:"column:id;primary_key"`
	CreatedAt *time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt *time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func NewBaseModelHardDelete() BaseModelHardDelete {
	return BaseModelHardDelete{
		ID:        utils.NewUUID(),
		CreatedAt: utils.NowPtr(),
		UpdatedAt: utils.NowPtr(),
	}
}
