package migrate

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"path/filepath"
	"testing"
)

func TestReadXUIOneClient(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	_, e := db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,up INTEGER,down INTEGER,total INTEGER,all_time INTEGER,remark TEXT,enable INTEGER,expiry_time INTEGER,listen TEXT,port INTEGER,protocol TEXT,settings TEXT,stream_settings TEXT,tag TEXT,sniffing TEXT);`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO inbounds VALUES(1,10,20,0,30,'Alpha',1,0,'',21001,'vless','{"clients":[{"id":"x","email":"alpha","totalGB":100}],"decryption":"none"}','{}','a','{}')`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	r, e := ReadXUI(p)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Accounts) != 1 || r.Accounts[0].Port != 21001 {
		t.Fatalf("%+v", r)
	}
}
func TestReadXUIMultiClientSkipped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,port INTEGER,protocol TEXT,settings TEXT);`)
	db.Exec(`INSERT INTO inbounds VALUES(1,21002,'vless','{"clients":[{"id":"a"},{"id":"b"}]}')`)
	db.Close()
	r, e := ReadXUI(p)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Accounts) != 0 || len(r.Skipped) != 1 {
		t.Fatalf("%+v", r)
	}
}
func TestReadXUILegacyColumns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,remark TEXT,enable INTEGER,port INTEGER,protocol TEXT,settings TEXT);`)
	db.Exec(`INSERT INTO inbounds VALUES(1,'Legacy',1,22001,'vless','{"clients":[{"id":"x"}]}')`)
	db.Close()
	r, e := ReadXUI(p)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Accounts) != 1 {
		t.Fatalf("%+v", r)
	}
}
