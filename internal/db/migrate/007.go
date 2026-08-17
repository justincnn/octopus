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
	RegisterAfterAutoMigration(Migration{
		Version: 8,
		Up:      migrateGroupItemStrategy,
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

// migrateGroupItemStrategy 存量分组 mode → item_strategy 映射:
// 轮询(1)→round_robin, 随机(2)→random, 故障转移(3)→priority, 加权(4)→least_used。
// 新列由 AutoMigrate 按 gorm tag 添加(group_items.enabled 默认 true = 存量全部启用, 行为不变)。
func migrateGroupItemStrategy(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("groups") || !db.Migrator().HasColumn("groups", "item_strategy") {
		return nil
	}
	return db.Exec(`
UPDATE groups
SET item_strategy = CASE mode
	WHEN 2 THEN 'random'
	WHEN 3 THEN 'priority'
	WHEN 4 THEN 'least_used'
	ELSE 'round_robin'
END
WHERE item_strategy = 'round_robin' AND mode != 1`).Error
}
