package store_test

import (
	"testing"

	"github.com/pskclub/mine-core-template/modules/user/store"
	"gorm.io/gorm"
)

// A blank q must hand the query back untouched, so callers can always pass the
// scope without checking first.
func TestSearch_blankQueryIsANoop(t *testing.T) {
	for _, q := range []string{"", "   ", "\t\n"} {
		db := &gorm.DB{}

		if got := store.Search(q)(db); got != db {
			t.Errorf("Search(%q) altered the query; a blank q must add no condition", q)
		}
	}
}
