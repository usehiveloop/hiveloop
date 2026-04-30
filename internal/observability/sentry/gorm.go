package sentry

import (
	"errors"

	sentrygo "github.com/getsentry/sentry-go"
	"gorm.io/gorm"
)

const gormSpanKey = "sentry:span"

func InstallGORMPlugin(db *gorm.DB) error {
	if !Enabled() || db == nil {
		return nil
	}

	cb := db.Callback()

	pairs := []struct {
		op    string
		label string
	}{
		{"create", "db.sql.create"},
		{"query", "db.sql.query"},
		{"update", "db.sql.update"},
		{"delete", "db.sql.delete"},
		{"row", "db.sql.row"},
		{"raw", "db.sql.raw"},
	}

	for _, pair := range pairs {
		op := pair.op
		label := pair.label
		var beforeReg func(string, func(*gorm.DB)) error
		var afterReg func(string, func(*gorm.DB)) error

		switch op {
		case "create":
			beforeReg = cb.Create().Before("gorm:create").Register
			afterReg = cb.Create().After("gorm:create").Register
		case "query":
			beforeReg = cb.Query().Before("gorm:query").Register
			afterReg = cb.Query().After("gorm:query").Register
		case "update":
			beforeReg = cb.Update().Before("gorm:update").Register
			afterReg = cb.Update().After("gorm:update").Register
		case "delete":
			beforeReg = cb.Delete().Before("gorm:delete").Register
			afterReg = cb.Delete().After("gorm:delete").Register
		case "row":
			beforeReg = cb.Row().Before("gorm:row").Register
			afterReg = cb.Row().After("gorm:row").Register
		case "raw":
			beforeReg = cb.Raw().Before("gorm:raw").Register
			afterReg = cb.Raw().After("gorm:raw").Register
		}

		if err := beforeReg("sentry:before_"+op, beforeCallback(label)); err != nil {
			return err
		}
		if err := afterReg("sentry:after_"+op, afterCallback()); err != nil {
			return err
		}
	}

	return nil
}

func beforeCallback(label string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		ctx := tx.Statement.Context
		if ctx == nil {
			return
		}
		// Skip stand-alone spans so AutoMigrate doesn't flood Sentry on boot.
		if sentrygo.TransactionFromContext(ctx) == nil {
			return
		}
		span := sentrygo.StartSpan(ctx, label)
		span.SetData("db.system", "postgresql")
		if tx.Statement.Table != "" {
			span.SetData("db.table", tx.Statement.Table)
		}
		tx.Statement.Settings.Store(gormSpanKey, span)
	}
}

func afterCallback() func(*gorm.DB) {
	return func(tx *gorm.DB) {
		raw, ok := tx.Statement.Settings.LoadAndDelete(gormSpanKey)
		if !ok {
			return
		}
		span, ok := raw.(*sentrygo.Span)
		if !ok || span == nil {
			return
		}
		span.Description = tx.Statement.SQL.String()
		span.SetData("db.rows_affected", tx.Statement.RowsAffected)
		if tx.Error != nil && !errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			span.Status = sentrygo.SpanStatusInternalError
			span.SetData("error", tx.Error.Error())
		} else {
			span.Status = sentrygo.SpanStatusOK
		}
		span.Finish()
	}
}
