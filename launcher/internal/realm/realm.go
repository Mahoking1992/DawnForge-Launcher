package realm

import (
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "strconv"
 "strings"
 "time"
)

const StatusURL = "https://dawnforge.eu/api/status"

type Snapshot struct {
 State string
 Players string
 Uptime string
}
func Unknown() Snapshot { return Snapshot{State:"UNKNOWN",Players:"—",Uptime:"—"} }
func statusWord(s string)string {
 switch strings.ToLower(strings.TrimSpace(s)){
 case "online","up","running","active":return "ONLINE"
 case "offline","down","stopped":return "OFFLINE"
 case "maintenance":return "MAINTENANCE"
 default:return "UNKNOWN"
 }
}
func fieldString(raw json.RawMessage) string {
 if len(raw)==0 || string(raw)=="null" {return ""}
 var s string
 if json.Unmarshal(raw,&s)==nil{return strings.TrimSpace(s)}
 var n json.Number
 if json.Unmarshal(raw,&n)==nil{return n.String()}
 return ""
}
func formattedUptime(seconds int64)string {
 if seconds<0{return "—"}
 days:=seconds/86400
 hours:=(seconds%86400)/3600
 minutes:=(seconds%3600)/60
 if days>0{return fmt.Sprintf("%dd %dh %dm",days,hours,minutes)}
 return fmt.Sprintf("%dh %dm",hours,minutes)
}
func decodeSeconds(raw json.RawMessage)(int64,bool){
 s:=fieldString(raw)
 if s==""{return 0,false}
 n,err:=strconv.ParseInt(s,10,64)
 return n,err==nil&&n>=0
}
func validCount(raw json.RawMessage)string{
 n,ok:=decodeSeconds(raw)
 if !ok || n>1000000{return "—"}
 return strconv.FormatInt(n,10)
}
// Parse only authoritative game-world fields; never infer realm availability
// from website or CDN reachability, which may be independent.
func Parse(b []byte)(Snapshot,error){
 s:=Unknown()
 if len(b)==0||len(b)>65536{return s,errors.New("invalid status payload size")}
 var obj map[string]json.RawMessage
 if err:=json.Unmarshal(b,&obj);err!=nil{return s,err}
 if obj==nil{return s,errors.New("empty status payload")}
 if v:=fieldString(obj["worldStatus"]);v!="" {s.State=statusWord(v)
 } else if v:=fieldString(obj["realmStatus"]);v!=""{s.State=statusWord(v)
 } else if v:=fieldString(obj["status"]);v!=""{s.State=statusWord(v)}
 for _,key:=range []string{"players","playersOnline","onlinePlayers"}{
  if v:=validCount(obj[key]);v!="—"{s.Players=v;break}
 }
 for _,key:=range []string{"uptimeSeconds","worldUptimeSeconds","uptime_seconds"}{
  if n,ok:=decodeSeconds(obj[key]);ok{s.Uptime=formattedUptime(n);break}
 }
 if s.Uptime=="—" {
  // A web status panel may expose an already-formatted uptime string.
  if v:=fieldString(obj["uptime"]);v!=""&&len(v)<40{
   if n,err:=strconv.ParseInt(v,10,64);err==nil{s.Uptime=formattedUptime(n)
   }else{s.Uptime=v}
  }
 }
 return s,nil
}
func Fetch(client *http.Client)(Snapshot,error){
 if client==nil{client=&http.Client{Timeout:5*time.Second}}
 req,err:=http.NewRequest("GET",StatusURL,nil);if err!=nil{return Unknown(),err}
 req.Header.Set("Accept","application/json")
 req.Header.Set("Cache-Control","no-cache")
 res,err:=client.Do(req);if err!=nil{return Unknown(),err}
 defer res.Body.Close()
 if res.StatusCode!=200{return Unknown(),fmt.Errorf("status API HTTP %d",res.StatusCode)}
 if !strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")),"application/json"){
  return Unknown(),errors.New("status API did not return JSON")
 }
 b,err:=io.ReadAll(io.LimitReader(res.Body,65537))
 if err!=nil{return Unknown(),err}
 return Parse(b)
}
