package db

import (
	"time"

	"github.com/adrianozp/platform/pkg/config"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewGormDB(cfg config.Config) (*gorm.DB, error) {
	dsn, err := withParseTime(cfg.DB.DSN)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Warn)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetConnMaxLifetime(3 * time.Minute)
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxOpenConns)
	return db, nil
}

func withParseTime(dsn string) (string, error) {
	c, err := driver.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	c.ParseTime = true
	c.Loc = time.UTC
	return c.FormatDSN(), nil
}
