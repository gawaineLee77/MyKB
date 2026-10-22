package agenticon

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestQuotaCheckIsSerializedWithUploadInOneTransaction(t *testing.T) {
	for _, full := range []bool{false, true} {
		db, m, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		i := Image{ID: "a", TenantID: 7, UserID: "u", PNG: []byte("png")}
		m.ExpectBegin()
		m.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`)).WithArgs("agent-icon:7:u").WillReturnResult(sqlmock.NewResult(0, 1))
		count := 0
		if full {
			count = 100
		}
		m.ExpectQuery(`SELECT count\(\*\),COALESCE`).WithArgs(7, "u").WillReturnRows(sqlmock.NewRows([]string{"count", "size"}).AddRow(count, 0))
		if full {
			m.ExpectRollback()
		} else {
			m.ExpectExec(`INSERT INTO mindcreek.agent_icons`).WithArgs(i.ID, 7, "u", i.PNG).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
		}
		err = NewRepository(db).Put(context.Background(), i)
		if full && err != ErrQuota || !full && err != nil {
			t.Fatal(err)
		}
		if err = m.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}
