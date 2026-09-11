package migrate

import (
	"database/sql"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func TestReadXUIOneClient(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	_, e := db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,up INTEGER,down INTEGER,total INTEGER,all_time INTEGER,remark TEXT,enable INTEGER,expiry_time INTEGER,listen TEXT,port INTEGER,protocol TEXT,settings TEXT,stream_settings TEXT,tag TEXT,sniffing TEXT);`)
	if e != nil { t.Fatal(e) }
	_, e = db.Exec(`INSERT INTO inbounds VALUES(1,10,20,0,30,'Alpha',1,0,'',21001,'vless','{"clients":[{"id":"x","email":"alpha","totalGB":100}],"decryption":"none"}','{}','a','{}')`)
	if e != nil { t.Fatal(e) }
	hash, _ := bcrypt.GenerateFromPassword([]byte("old-panel-password"), bcrypt.DefaultCost)
	if _, e = db.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, username TEXT, password TEXT);`); e != nil { t.Fatal(e) }
	if _, e = db.Exec(`INSERT INTO users VALUES(1,?,?)`, "legacy-admin", string(hash)); e != nil { t.Fatal(e) }
	if _, e = db.Exec(`CREATE TABLE settings(id INTEGER PRIMARY KEY, key TEXT, value TEXT);`); e != nil { t.Fatal(e) }
	if _, e = db.Exec(`INSERT INTO settings(key,value) VALUES
('webListen',''),('webPort','13688'),('webBasePath','legacy-panel'),
('webDomain','panel.example.com'),('webCertFile','/root/cert/panel/fullchain.pem'),('webKeyFile','/root/cert/panel/privkey.pem')`); e != nil { t.Fatal(e) }
	db.Close()

	r, e := ReadXUI(p)
	if e != nil { t.Fatal(e) }
	if len(r.Accounts) != 1 || r.Accounts[0].Port != 21001 { t.Fatalf("%+v", r) }
	if len(r.Admins) != 1 || r.Admins[0].Username != "legacy-admin" { t.Fatalf("admins: %+v", r.Admins) }
	if bcrypt.CompareHashAndPassword([]byte(r.Admins[0].PasswordHash), []byte("old-panel-password")) != nil { t.Fatal("migrated bcrypt hash does not preserve the old password") }
	if r.PanelListen != ":13688" { t.Fatalf("panel listen: %q", r.PanelListen) }
	if r.PanelBasePath != "/legacy-panel/" { t.Fatalf("base path: %q", r.PanelBasePath) }
	if r.PanelDomain != "panel.example.com" { t.Fatalf("domain: %q", r.PanelDomain) }
	if r.PanelCertFile != "/root/cert/panel/fullchain.pem" || r.PanelKeyFile != "/root/cert/panel/privkey.pem" { t.Fatalf("TLS paths: cert=%q key=%q", r.PanelCertFile, r.PanelKeyFile) }
}

func TestReadXUIPartialTLSWarnsAndDoesNotEnableTLS(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	_, _ = db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,port INTEGER,protocol TEXT,settings TEXT);`)
	_, _ = db.Exec(`INSERT INTO inbounds VALUES(1,21001,'vless','{"clients":[{"id":"x"}]}')`)
	_, _ = db.Exec(`CREATE TABLE settings(id INTEGER PRIMARY KEY, key TEXT, value TEXT);`)
	_, _ = db.Exec(`INSERT INTO settings(key,value) VALUES('webPort','13688'),('webCertFile','/root/cert/panel/fullchain.pem')`)
	db.Close()
	r, err := ReadXUI(p)
	if err != nil { t.Fatal(err) }
	if r.PanelCertFile != "" || r.PanelKeyFile != "" { t.Fatalf("partial TLS should not migrate: %+v", r) }
	if len(r.Warnings) == 0 { t.Fatal("expected TLS warning") }
}

func TestReadXUIMultiClientSkipped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,port INTEGER,protocol TEXT,settings TEXT);`)
	db.Exec(`INSERT INTO inbounds VALUES(1,21002,'vless','{"clients":[{"id":"a"},{"id":"b"}]}')`)
	db.Close()
	r, e := ReadXUI(p)
	if e != nil { t.Fatal(e) }
	if len(r.Accounts) != 0 || len(r.Skipped) != 1 { t.Fatalf("%+v", r) }
}

func TestReadXUILegacyColumns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.db")
	db, _ := sql.Open("sqlite", p)
	db.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,remark TEXT,enable INTEGER,port INTEGER,protocol TEXT,settings TEXT);`)
	db.Exec(`INSERT INTO inbounds VALUES(1,'Legacy',1,22001,'vless','{"clients":[{"id":"x"}]}')`)
	db.Close()
	r, e := ReadXUI(p)
	if e != nil { t.Fatal(e) }
	if len(r.Accounts) != 1 { t.Fatalf("%+v", r) }
}
