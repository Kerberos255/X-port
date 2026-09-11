package backup

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kerberos255/X-port/internal/store"
)

type Info struct { Name string `json:"name"`; Size int64 `json:"size"`; CreatedAt int64 `json:"createdAt"` }
type File struct { Format string `json:"format"`; CreatedAt int64 `json:"createdAt"`; Snapshot store.Snapshot `json:"snapshot"` }

func Create(dir string, snapshot store.Snapshot)(Info,error){
	if err:=os.MkdirAll(dir,0750);err!=nil{return Info{},err}
	now:=time.Now();name:="xport-"+now.Format("20060102-150405.000")+".json.gz";path:=filepath.Join(dir,name)
	f,err:=os.OpenFile(path+".tmp",os.O_CREATE|os.O_WRONLY|os.O_TRUNC,0600);if err!=nil{return Info{},err}
	gz:=gzip.NewWriter(f);enc:=json.NewEncoder(gz);enc.SetEscapeHTML(false);writeErr:=enc.Encode(File{Format:"xport-backup-v1",CreatedAt:now.UnixMilli(),Snapshot:snapshot});closeGZ:=gz.Close();closeFile:=f.Close()
	if writeErr!=nil||closeGZ!=nil||closeFile!=nil{_ = os.Remove(path+".tmp");if writeErr!=nil{return Info{},writeErr};if closeGZ!=nil{return Info{},closeGZ};return Info{},closeFile}
	if err:=os.Rename(path+".tmp",path);err!=nil{_ = os.Remove(path+".tmp");return Info{},err};st,err:=os.Stat(path);if err!=nil{return Info{},err};return Info{Name:name,Size:st.Size(),CreatedAt:now.UnixMilli()},nil
}
func List(dir string)([]Info,error){entries,err:=os.ReadDir(dir);if errors.Is(err,os.ErrNotExist){return []Info{},nil};if err!=nil{return nil,err};out:=make([]Info,0,len(entries));for _,e:=range entries{if e.IsDir()||!strings.HasSuffix(e.Name(),".json.gz"){continue};st,err:=e.Info();if err!=nil{continue};out=append(out,Info{Name:e.Name(),Size:st.Size(),CreatedAt:st.ModTime().UnixMilli()})};sort.Slice(out,func(i,j int)bool{return out[i].CreatedAt>out[j].CreatedAt});return out,nil}
func Load(dir,name string)(store.Snapshot,error){path,err:=safePath(dir,name);if err!=nil{return store.Snapshot{},err};f,err:=os.Open(path);if err!=nil{return store.Snapshot{},err};defer f.Close();gz,err:=gzip.NewReader(io.LimitReader(f,64<<20));if err!=nil{return store.Snapshot{},err};defer gz.Close();var payload File;if err:=json.NewDecoder(io.LimitReader(gz,64<<20)).Decode(&payload);err!=nil{return store.Snapshot{},err};if payload.Format!="xport-backup-v1"||payload.Snapshot.Version!=1{return store.Snapshot{},errors.New("unsupported X-port backup format")};return payload.Snapshot,nil}
func Delete(dir,name string)error{path,err:=safePath(dir,name);if err!=nil{return err};return os.Remove(path)}
func Open(dir,name string)(*os.File,Info,error){path,err:=safePath(dir,name);if err!=nil{return nil,Info{},err};f,err:=os.Open(path);if err!=nil{return nil,Info{},err};st,err:=f.Stat();if err!=nil{f.Close();return nil,Info{},err};return f,Info{Name:name,Size:st.Size(),CreatedAt:st.ModTime().UnixMilli()},nil}
func safePath(dir,name string)(string,error){if name==""||filepath.Base(name)!=name||!strings.HasSuffix(name,".json.gz")||strings.Contains(name,".."){return "",fmt.Errorf("invalid backup name")};return filepath.Join(dir,name),nil}
