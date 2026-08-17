package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 6,
		Up:      addChannelKeysColumn,
	})
}

// addChannelKeysColumn 给 channels 表加 keys 列(多 key 轮询池, JSON 数组)。
func addChannelKeysColumn(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("channels") {
		return nil
	}
	if db.Migrator().HasColumn("channels", "keys") {
		return nil
	}
	return db.Migrator().AddColumn(&model.Channel{}, "Keys")
}
