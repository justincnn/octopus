package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 7,
		Up:      addChannelKeyStrategyColumn,
	})
}

// addChannelKeyStrategyColumn 给 channels 表加 key_strategy 列(轮询策略, 默认 priority)。
func addChannelKeyStrategyColumn(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("channels") {
		return nil
	}
	if db.Migrator().HasColumn("channels", "key_strategy") {
		return nil
	}
	return db.Migrator().AddColumn(&model.Channel{}, "KeyStrategy")
}
