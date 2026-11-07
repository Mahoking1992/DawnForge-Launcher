package realm

import "testing"

func TestStatusOnline(t *testing.T){
 s,e:=Parse([]byte(`{"worldStatus":"online","loginStatus":"offline","players":293,"uptimeSeconds":38640}`))
 if e!=nil||s.State!="ONLINE"||s.Players!="293"||s.Uptime!="10h 44m" {t.Fatalf("%+v %v",s,e)}
}
func TestWorldStatusHasPrecedence(t *testing.T){
 s,e:=Parse([]byte(`{"worldStatus":"offline","status":"online","players":0,"uptimeSeconds":0}`))
 if e!=nil||s.State!="OFFLINE"||s.Players!="0" {t.Fatalf("%+v %v",s,e)}
}
func TestNoFakeOnline(t *testing.T){
 s,e:=Parse([]byte(`{"loginStatus":"online","api":"online"}`))
 if e!=nil||s.State!="UNKNOWN"||s.Players!="—"||s.Uptime!="—"{t.Fatalf("%+v %v",s,e)}
}
func TestStringValues(t *testing.T){
 s,e:=Parse([]byte(`{"status":"maintenance","players":"12","uptime":"2d 3h"}`))
 if e!=nil||s.State!="MAINTENANCE"||s.Players!="12"||s.Uptime!="2d 3h"{t.Fatalf("%+v %v",s,e)}
}
func TestMalformed(t *testing.T){if _,e:=Parse([]byte("oops"));e==nil{t.Fatal("must reject malformed JSON")}}
